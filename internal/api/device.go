package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/attribution"
	"github.com/hurshnarayan/reyna/internal/model"
)

// deviceIdentity is the phone value an on-device install registers under.
//
// The backend identifies group members by phone number, which a phone watching
// its own folders does not have and should not ask for. One stable value keeps
// uploads, questions and Drive on a single user record.
const deviceIdentity = "device"

// classifyingPlaceholder is the subject a file carries while the model reads it.
const classifyingPlaceholder = "classifying..."

// The API the Android app talks to.
//
// The app attributes at capture time so it can show a result immediately, and
// sends what it worked out. The server re-runs the join whenever new events
// arrive, because an event learned on one device can explain a file captured
// earlier, and the join has to run in both directions for that to work.

// deviceEvent is one message the phone learned about, from a notification it
// saw or a line in a chat export.
type deviceEvent struct {
	ChatKey        string `json:"chat_key"`
	ChatName       string `json:"chat_name"`
	SenderName     string `json:"sender_name"`
	PostedAt       int64  `json:"posted_at"` // epoch seconds
	Text           string `json:"text"`
	AttachmentName string `json:"attachment_name"`
	HasAttachment  bool   `json:"has_attachment"`
	Source         string `json:"source"`
}

func (d deviceEvent) toEvent() attribution.Event {
	return attribution.Event{
		ChatKey:        d.ChatKey,
		ChatName:       d.ChatName,
		SenderDisplay:  d.SenderName,
		PostedAt:       time.Unix(d.PostedAt, 0).UTC(),
		Text:           d.Text,
		AttachmentName: d.AttachmentName,
		HasAttachment:  d.HasAttachment || d.AttachmentName != "",
		Source:         d.Source,
	}
}

// handleDeviceEvents ingests a batch of observed messages and re-joins.
//
// Batched because the phone accumulates events while offline and sending one
// request per notification would be both slow and a good way to get rate
// limited. Duplicates collapse on the unique index, so a client that resends
// after a failed request costs nothing.
func (s *Server) handleDeviceEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	var req struct {
		GroupWAID string        `json:"group_wa_id"`
		Events    []deviceEvent `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}

	groupID := s.groupIDFor(req.GroupWAID)
	stored := 0
	for _, e := range req.Events {
		if e.PostedAt <= 0 {
			continue
		}
		id, err := s.store.InsertEvent(e.toEvent(), groupID)
		if err != nil {
			log.Printf("[DEVICE] event insert: %v", err)
			continue
		}
		if id > 0 {
			stored++
		}
	}

	improved := s.rejoinWeak(groupID)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"received": len(req.Events),
		"stored":   stored,
		"improved": improved,
	})
}

// handleDeviceExport ingests attachment rows parsed from a chat export.
//
// The phone parses the export and sends only the attachment rows: the raw chat
// text never leaves the device, which is what makes the import promise
// checkable rather than a claim.
func (s *Server) handleDeviceExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	var req struct {
		GroupWAID string        `json:"group_wa_id"`
		ChatName  string        `json:"chat_name"`
		Events    []deviceEvent `json:"events"`
		// Whether the day/month order was proven by the file or assumed. An
		// assumed order shifts every date by up to eleven months, so it is
		// recorded rather than silently accepted.
		DateOrderProven bool `json:"date_order_proven"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}

	groupID := s.groupIDFor(req.GroupWAID)
	stored := 0
	for _, e := range req.Events {
		if e.PostedAt <= 0 {
			continue
		}
		ev := e.toEvent()
		ev.Source = model.AttrExport
		if ev.ChatName == "" {
			ev.ChatName = req.ChatName
		}
		id, err := s.store.InsertEvent(ev, groupID)
		if err != nil {
			continue
		}
		if id > 0 {
			stored++
		}
	}

	improved := s.rejoinWeak(groupID)
	if !req.DateOrderProven {
		log.Printf("[DEVICE] export for group %d used an assumed date order", groupID)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"received":          len(req.Events),
		"stored":            stored,
		"improved":          improved,
		"date_order_proven": req.DateOrderProven,
	})
}

// handleDevicePending lists files Reyna cannot name, for the repair queue.
func (s *Server) handleDevicePending(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	files, err := s.store.WeaklyAttributed(nil, limit)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, 500)
		return
	}
	if files == nil {
		files = []model.File{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"files":     files,
		"threshold": model.AttributionMinNamed,
	})
}

// handleDeviceAttribute records the user telling us who shared a file.
//
// Full confidence, because they know and we do not.
func (s *Server) handleDeviceAttribute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	var req struct {
		FileID int64  `json:"file_id"`
		Sender string `json:"sender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == 0 || req.Sender == "" {
		http.Error(w, `{"error":"file_id and sender required"}`, 400)
		return
	}
	if err := s.store.SetSenderManually(req.FileID, req.Sender); err != nil {
		http.Error(w, `{"error":"update failed"}`, 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// handleDeviceDriveConnect hands the app a Google OAuth URL.
//
// The app cannot complete OAuth itself: the client secret lives on the server
// and shipping it in an APK would put it in every user's hands. So the app
// opens this URL in a browser, the user consents, and the callback lands back
// on the server, which stores the tokens.
func (s *Server) handleDeviceDriveConnect(w http.ResponseWriter, r *http.Request) {
	if !s.drive.IsConfigured() {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"configured": false,
			"message":    "Set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET on the server",
		})
		return
	}
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		phone = "device"
	}
	user, err := s.store.UpsertUser(phone, "")
	if err != nil {
		http.Error(w, `{"error":"could not identify device"}`, 500)
		return
	}
	// The state is the user's own signed token, validated on the way back, so
	// a callback cannot be replayed against a different account.
	token, _ := authGenerate(s, user.ID)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"configured": true,
		"url":        s.drive.GetAuthURL(token),
	})
}

// handleDeviceDriveStatus reports whether Drive is connected for this device.
func (s *Server) handleDeviceDriveStatus(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		phone = "device"
	}
	user, err := s.store.GetUserByPhone(phone)
	if err != nil || user == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"connected": false})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": user.GoogleRefresh != "",
		"email":     user.Email,
	})
}

// ── helpers ──

func (s *Server) groupIDFor(waID string) int64 {
	if waID == "" {
		return 0
	}
	group, err := s.store.GetGroupByWAID(waID)
	if err != nil || group == nil {
		s.store.InsertGroupRaw(waID, waID, 0)
		group, err = s.store.GetGroupByWAID(waID)
		if err != nil || group == nil {
			return 0
		}
	}
	return group.ID
}

// rejoinWeak re-runs the join over files we cannot yet name.
//
// Called whenever new events arrive, because an event learned now can explain
// a file captured hours ago. Only the weak ones are touched: a file attributed
// at full confidence has nothing to gain, and re-deciding it risks replacing a
// fact with a guess.
func (s *Server) rejoinWeak(groupID int64) int {
	var groupIDs []int64
	if groupID > 0 {
		groupIDs = []int64{groupID}
	}
	files, err := s.store.WeaklyAttributed(groupIDs, 200)
	if err != nil {
		return 0
	}

	improved := 0
	for _, f := range files {
		at := f.SharedAt()
		events, err := s.store.EventsAround(groupIDs, at, 3*24*time.Hour)
		if err != nil || len(events) == 0 {
			continue
		}

		result := attribution.Attribute(
			attribution.File{ID: f.ID, DiskName: f.FileName, MTime: at},
			events,
			time.Local,
		)
		if result.Confidence() <= f.AttributionConfidence {
			continue
		}

		var sender, chat string
		postedAt := at
		if result.Best != nil {
			for _, e := range events {
				if e.ID == result.Best.EventID {
					sender, chat = e.SenderDisplay, e.ChatName
					// The event's time is the real message time; the file's own
					// is only when it reached the disk.
					postedAt = e.PostedAt
					break
				}
			}
		}

		s.store.SaveLinks(f.ID, result)
		if err := s.store.ApplyAttribution(f.ID, sender, chat, postedAt, result); err == nil {
			improved++
		}
	}
	if improved > 0 {
		log.Printf("[DEVICE] re-join improved %d file(s)", improved)
	}
	return improved
}

// handleDeviceDriveState reports what is waiting to reach Drive.
//
// The phone knows a file left the device but not whether it ever reached the
// user's Drive: upload and commit are separate steps and commit runs on a
// timer. Without this the app could only claim everything was fine, which is
// exactly the claim it is least entitled to make.
func (s *Server) handleDeviceDriveState(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		phone = deviceIdentity
	}
	user, err := s.store.GetUserByPhone(phone)
	if err != nil || user == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected": false, "pending": 0, "in_drive": 0,
		})
		return
	}

	pending, inDrive := 0, 0
	names := []string{}
	for _, gid := range s.store.GetUserGroupIDs(user.ID) {
		staged, _ := s.store.GetStagedFiles(gid)
		for _, f := range staged {
			// Still being read by the model. Counting it as pending would show
			// a number that ticks up and down on its own.
			if f.Subject == classifyingPlaceholder {
				continue
			}
			pending++
			if len(names) < 5 {
				names = append(names, f.FileName)
			}
		}
		inDrive += s.store.CountCommittedFiles(gid)
	}

	connected := user.GoogleRefresh != "" && user.DriveRootID != "" &&
		!strings.HasPrefix(user.DriveRootID, "local_")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": connected,
		"email":     user.Email,
		"pending":   pending,
		"in_drive":  inDrive,
		"examples":  names,
	})
}

// handleDeviceDrivePush commits now instead of waiting for the timer.
func (s *Server) handleDeviceDrivePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		phone = deviceIdentity
	}
	user, err := s.store.GetUserByPhone(phone)
	if err != nil || user == nil {
		http.Error(w, `{"error":"unknown device"}`, 404)
		return
	}
	if user.GoogleRefresh == "" || user.DriveRootID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected": false, "uploaded": 0,
		})
		return
	}
	committed, uploaded := s.commitStagedForUser(user.ID)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": true,
		"committed": committed,
		"uploaded":  uploaded,
	})
}

// handleDeviceDriveDisconnect forgets the user's Google tokens.
//
// Nothing in Drive is touched. Disconnecting is a statement about what Reyna
// is allowed to do next, not an instruction to remove what it already filed,
// and deleting someone's documents because they revoked an integration would
// be indefensible. Files already in Drive stay exactly where they are.
func (s *Server) handleDeviceDriveDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		phone = deviceIdentity
	}
	user, err := s.store.GetUserByPhone(phone)
	if err != nil || user == nil {
		http.Error(w, `{"error":"unknown device"}`, 404)
		return
	}
	s.store.ClearUserGoogle(user.ID)
	log.Printf("[DRIVE] disconnected for device user %d", user.ID)
	json.NewEncoder(w).Encode(map[string]interface{}{"disconnected": true})
}
