package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/nlp"
)

func (s *Server) handleBotCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req model.CommandRequest
	json.NewDecoder(r.Body).Decode(&req)

	action, args := s.reyna.ProcessCommand(req.Command)
	userName := req.UserName
	if userName == "" { userName = req.UserPhone }
	user, _ := s.store.UpsertUser(req.UserPhone, userName)

	group, err := s.store.GetGroupByWAID(req.GroupWAID)
	if err != nil { group, _ = s.store.UpsertGroup(req.GroupWAID, "WhatsApp Group", user.ID) }
	if group != nil { s.store.AddGroupMember(group.ID, user.ID, req.UserPhone, "member") }
	groupID := int64(0)
	if group != nil { groupID = group.ID }

	resp := model.CommandResponse{}

	switch action {
	case "add":
		// Only metadata add (no file data) — used as fallback
		fileName := args
		// Parse --folder / -f flag
		folderFlag := ""
		if strings.Contains(args, "--folder ") {
			parts := strings.SplitN(args, "--folder ", 2)
			fileName = strings.TrimSpace(parts[0])
			folderFlag = strings.TrimSpace(parts[1])
		} else if strings.Contains(args, "-f ") {
			parts := strings.SplitN(args, "-f ", 2)
			fileName = strings.TrimSpace(parts[0])
			folderFlag = strings.TrimSpace(parts[1])
		}
		if fileName == "" || fileName == "." { fileName = req.FileName }
		if fileName == "" { fileName = "unknown_file" }
		subject := req.Subject
		if folderFlag != "" { subject = folderFlag }
		file := &model.File{GroupID: groupID, UserID: user.ID, SharedByPhone: req.UserPhone, SharedByName: req.UserName, FileName: fileName, FileSize: req.FileSize, MimeType: req.MimeType, Subject: subject, DriveFileID: fmt.Sprintf("meta_%d", time.Now().UnixNano()), Status: "staged"}
		saved, _ := s.store.AddFile(file)
		total := s.store.CountGroupFiles(groupID)
		v := 1; if saved != nil { v = saved.Version }
		resp.Reply = s.reyna.AddResponse(fileName, v, total) + s.reyna.AddReminder()

	case "commit":
		// Parse -m flag for commit message
		commitMsg := ""
		commitFileName := args
		if strings.Contains(args, "-m ") {
			parts := strings.SplitN(args, "-m ", 2)
			commitFileName = strings.TrimSpace(parts[0])
			commitMsg = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		}
		// Parse --folder / -f flag
		commitFolder := ""
		if strings.Contains(commitFileName, "--folder ") {
			parts := strings.SplitN(commitFileName, "--folder ", 2)
			commitFileName = strings.TrimSpace(parts[0])
			commitFolder = strings.TrimSpace(parts[1])
		} else if strings.Contains(commitFileName, "-f ") {
			parts := strings.SplitN(commitFileName, "-f ", 2)
			commitFileName = strings.TrimSpace(parts[0])
			commitFolder = strings.TrimSpace(parts[1])
		}
		_ = commitFolder // Used for folder override during upload

		staged, _ := s.store.GetStagedFiles(groupID)
		log.Printf("[COMMIT] staged=%d group=%d msg=%q", len(staged), groupID, commitMsg)
		if len(staged) == 0 {
			resp.Reply = s.reyna.CommitEmptyResponse()
		} else {
			driveUser := s.store.FindDriveConnectedUser(groupID)
			if driveUser != nil {
				log.Printf("[COMMIT] drive_user=%s id=%d has_refresh=%v root=%s configured=%v",
					driveUser.Email, driveUser.ID, driveUser.GoogleRefresh != "", driveUser.DriveRootID, s.drive.IsConfigured())
			} else {
				log.Printf("[COMMIT] NO drive user found")
			}
			uploaded := 0
			skipped := 0
			for i, f := range staged {
				log.Printf("[COMMIT][%d] file=%s id=%d size=%d subject=%s", i, f.FileName, f.ID, f.FileSize, f.Subject)

				if f.Subject == "classifying..." {
					log.Printf("[COMMIT][%d] SKIP still classifying", i)
					skipped++
					continue
				}

				canUpload := driveUser != nil && driveUser.GoogleRefresh != "" && s.drive.IsConfigured() && driveUser.DriveRootID != "" && !strings.HasPrefix(driveUser.DriveRootID, "local_")
				log.Printf("[COMMIT][%d] canUpload=%v", i, canUpload)

				if !canUpload {
					log.Printf("[COMMIT][%d] SKIP no drive", i)
					skipped++
					continue
				}

				token, terr := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
				if terr != nil {
					log.Printf("[COMMIT][%d] SKIP token error: %v", i, terr)
					skipped++
					continue
				}
				log.Printf("[COMMIT][%d] token_len=%d", i, len(token))
				s.store.UpdateUserGoogle(driveUser.ID, driveUser.Email, token, driveUser.GoogleRefresh, driveUser.DriveRootID)

				// Use commit folder override if provided, otherwise use file subject
				subjectForFolder := f.Subject
				if commitFolder != "" { subjectForFolder = commitFolder }
				folderID, ferr := s.drive.EnsureSubjectFolder(token, driveUser.DriveRootID, subjectForFolder)
				if ferr != nil {
					log.Printf("[COMMIT][%d] SKIP folder error: %v", i, ferr)
					skipped++
					continue
				}
				log.Printf("[COMMIT][%d] folder=%s", i, folderID)

				fileData, rerr := s.drive.GetLocalFileData(f.ID)
				log.Printf("[COMMIT][%d] raw_data len=%d err=%v", i, len(fileData), rerr)
				if len(fileData) == 0 {
					fileData, _ = s.drive.GetFileFromLocalStore(f.UserID, f.Subject, f.FileName)
					log.Printf("[COMMIT][%d] fallback len=%d", i, len(fileData))
				}
				if len(fileData) == 0 {
					log.Printf("[COMMIT][%d] SKIP no data", i)
					skipped++
					continue
				}

				log.Printf("[COMMIT][%d] uploading %d bytes...", i, len(fileData))
				driveID, uerr := s.drive.UploadFileToDrive(token, folderID, f.FileName, f.MimeType, fileData)
				if uerr != nil {
					log.Printf("[COMMIT][%d] SKIP upload error: %v", i, uerr)
					skipped++
					continue
				}
				if driveID != "" {
					s.store.UpdateFileDriveID(f.ID, driveID, folderID)
					uploaded++
					log.Printf("[COMMIT][%d] SUCCESS drive_id=%s", i, driveID)
				} else {
					log.Printf("[COMMIT][%d] SKIP empty drive id", i)
					skipped++
				}
			}
			count, _ := s.store.CommitFiles(groupID)
			log.Printf("[COMMIT] DONE committed=%d uploaded=%d skipped=%d", count, uploaded, skipped)
			if commitMsg != "" {
				resp.Reply = s.reyna.CommitWithMessageResponse(int(count), commitMsg)
			} else if commitFileName != "" {
				s.store.CommitFileByName(groupID, commitFileName)
				resp.Reply = s.reyna.CommitFileResponse(commitFileName)
			} else {
				resp.Reply = s.reyna.CommitAllResponse(int(count))
			}
			if uploaded > 0 { resp.Reply += fmt.Sprintf("\n\n☁️ %d file(s) pushed to Google Drive!", uploaded) }
			if skipped > 0 && uploaded == 0 {
				if driveUser == nil { resp.Reply += "\n\nNo one in this group has connected Drive yet. Files are saved locally. Connect Drive on the dashboard to sync." } else { resp.Reply += "\n\nDrive upload failed. Check backend logs." }
			}
		}

	case "rm":
		if args == "" || args == "." {
			count, _ := s.store.RemoveAllStaged(groupID)
			if count == 0 { resp.Reply = s.reyna.RmEmptyResponse() } else { resp.Reply = s.reyna.RmAllResponse(int(count)) }
		} else {
			ok, _ := s.store.RemoveStagedFile(groupID, args)
			if ok { resp.Reply = s.reyna.RmFileResponse(args) } else { resp.Reply = s.reyna.RmNotFoundResponse(args) }
		}

	case "staged":
		staged, _ := s.store.GetStagedFiles(groupID)
		if staged == nil { staged = []model.File{} }
		resp.Files = staged
		resp.Reply = s.reyna.StagedResponse(staged)

	case "find":
		if args == "" { resp.Reply = "Find kya? Usage: /reyna find \"DSA notes\" 🤦‍♀️"; break }
		// Parse -a flag for author details
		showAuthor := false
		query := args
		if strings.Contains(args, " -a") {
			showAuthor = true
			query = strings.TrimSpace(strings.Replace(args, " -a", "", 1))
			query = strings.TrimSpace(strings.Replace(query, "-a ", "", 1))
		}
		files, _ := s.store.FindFiles(groupID, query)
		if files == nil { files = []model.File{} }
		resp.Files = files
		if showAuthor {
			resp.Reply = s.reyna.FindWithAuthorResponse(query, files)
		} else {
			resp.Reply = s.reyna.FindResponse(query, files)
		}

	case "log":
		files, _ := s.store.GetGroupFiles(groupID, 10)
		if files == nil { files = []model.File{} }
		total := s.store.CountGroupFiles(groupID)
		resp.Files = files; resp.LogCount = total
		resp.Reply = s.reyna.LogResponse(files, total)

	case "status":
		staged, _ := s.store.GetStagedFiles(groupID)
		total := s.store.CountGroupFiles(groupID)
		// Return git-style status
		resp.Files = staged
		resp.Reply = s.reyna.StatusResponseGit(staged, nil, total-len(staged), total)

	case "help":
		resp.Reply = s.reyna.HelpResponse()

	case "enable":
		if groupID > 0 {
			gs := s.store.GetGroupSettings(groupID)
			gs.GroupID = groupID
			gs.Enabled = true
			gs.Hidden = false // un-hide so it reappears in dashboard
			s.store.UpsertGroupSettings(gs)
			log.Printf("[BOT] Enabled tracking for group %d via /reyna init", groupID)
			resp.Reply = "Tracking enabled."
		} else {
			resp.Reply = "Group not found."
		}

	case "disable":
		if groupID > 0 {
			gs := s.store.GetGroupSettings(groupID)
			gs.GroupID = groupID
			gs.Enabled = false
			s.store.UpsertGroupSettings(gs)
			log.Printf("[BOT] Disabled tracking for group %d via /reyna stop", groupID)
			resp.Reply = "Tracking disabled."
		} else {
			resp.Reply = "Group not found."
		}

	case "mode":
		if groupID > 0 {
			gs := s.store.GetGroupSettings(groupID)
			targetMode := strings.ToLower(strings.TrimSpace(args))
			if targetMode == "reaction" || targetMode == "reactions" || targetMode == "react" {
				gs.TrackingMode = "reaction"
				s.store.UpsertGroupSettings(gs)
				emoji := gs.ReactionEmoji
				if emoji == "" { emoji = "📌" }
				resp.Reply = fmt.Sprintf("Tracking mode switched to *reactions only*. %s\n\nFiles shared in this group will only be staged when someone reacts with %s.", emoji, emoji)
			} else if targetMode == "auto" || targetMode == "all" || targetMode == "track all" {
				gs.TrackingMode = "auto"
				s.store.UpsertGroupSettings(gs)
				resp.Reply = "Tracking mode switched to *track all files*. 📂\n\nEvery document shared in this group will now be automatically staged and synced."
			} else {
				resp.Reply = fmt.Sprintf("Current tracking mode: *%s*.\n\nUsage: `/reyna mode auto` or `/reyna mode reaction`", gs.TrackingMode)
			}
		} else {
			resp.Reply = "Group not found."
		}

	case "tracking":
		// Handled by bot directly, but provide a fallback
		resp.Reply = "📊 Use this in WhatsApp to see tracked/untracked files in real time."

	default:
		// Try to give a helpful suggestion
		parts := strings.Fields(req.Command)
		attempted := ""
		if len(parts) > 1 { attempted = parts[1] }
		if attempted != "" {
			resp.Reply = s.reyna.InvalidCommandResponse(attempted)
		} else {
			resp.Reply = s.reyna.GenericResponse()
		}
	}

	s.store.LogActivity(groupID, user.ID, action, req.Command, "")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleBotUpload(w http.ResponseWriter, r *http.Request) {
	// CORS
	origin := r.Header.Get("Origin")
	if origin == "" { origin = "*" }
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "OPTIONS" { w.WriteHeader(200); return }
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }

	// 50MB max
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		log.Printf("❌ ParseMultipartForm: %v", err)
		http.Error(w, `{"error":"parse error: `+err.Error()+`"}`, 400)
		return
	}

	groupWAID := r.FormValue("group_wa_id")
	userPhone := r.FormValue("user_phone")
	userName := r.FormValue("user_name")
	fileName := r.FormValue("file_name")
	mimeType := r.FormValue("mime_type")
	subject := r.FormValue("subject")
	fileSizeStr := r.FormValue("file_size")
	fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)

	// When the message was actually sent, epoch seconds, from the WhatsApp
	// envelope. Zero when the client did not supply it, in which case the row
	// keeps a NULL posted_at and reads fall back to the insert time.
	postedAt := time.Time{}
	if v, err := strconv.ParseInt(r.FormValue("posted_at"), 10, 64); err == nil && v > 0 {
		postedAt = time.Unix(v, 0).UTC()
	}

	// Read uploaded file
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Printf("❌ FormFile error: %v", err)
		http.Error(w, `{"error":"file missing from form"}`, 400)
		return
	}
	defer file.Close()
	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		log.Printf("❌ Read file error: %v (len=%d)", err, len(fileBytes))
		http.Error(w, `{"error":"empty file"}`, 400)
		return
	}

	if fileSize == 0 { fileSize = int64(len(fileBytes)) }
	log.Printf("📤 Bot upload: %s (%d bytes) from %s (phone=%s)", fileName, len(fileBytes), userName, userPhone)

	// ── Content-hash duplicate detection ──
	// Compute SHA-256 of the file bytes BEFORE we do anything expensive. If
	// this exact file already exists in the same WhatsApp group, dismiss the
	// upload as a duplicate (don't classify, don't extract, don't push to
	// Drive). Same hash + different filename → still a duplicate. Same name
	// + different hash → genuine update, save as v2.
	hashSum := sha256.Sum256(fileBytes)
	contentHash := hex.EncodeToString(hashSum[:])
	log.Printf("🔑 hash=%s for %s", contentHash[:12], fileName)

	// We need the group resolved before the dedup check. Resolve the user +
	// group here, then run the dedup check, then continue with classification.
	sharedByName := userName // preserve original name (pushName from WhatsApp)
	// For the user record, use pushName if available, otherwise phone
	userRecordName := userName
	if userRecordName == "" { userRecordName = userPhone }
	user, _ := s.store.UpsertUser(userPhone, userRecordName)

	// If shared_by_name is still empty, try to get it from an existing user record
	if sharedByName == "" && user != nil && user.Name != "" {
		// Only use stored name if it doesn't look like a phone/LID
		stored := strings.TrimSpace(user.Name)
		if stored != "" && !looksLikePhoneOrLID(stored) {
			sharedByName = stored
		}
	}

	group, err := s.store.GetGroupByWAID(groupWAID)
	if err != nil { group, _ = s.store.UpsertGroup(groupWAID, "WhatsApp Group", user.ID) }
	if group != nil { s.store.AddGroupMember(group.ID, user.ID, userPhone, "member") }
	groupID := int64(0)
	if group != nil { groupID = group.ID }

	// ── Per-group serialization lock ──
	// Without this, simultaneous uploads of byte-identical files all pass the
	// dedup check before any of them inserts, leading to N copies of the same
	// file landing in the staging area. The lock makes the check + insert
	// atomic for one group at a time.
	uploadMu := s.uploadLockFor(groupID)
	uploadMu.Lock()
	defer uploadMu.Unlock()

	// ── Dedup check: same hash already in this group? ──
	if existing := s.store.FindFileByHash(groupID, contentHash); existing != nil {
		log.Printf("♻️  Duplicate ignored: %s (matches existing %q, id=%d)", fileName, existing.FileName, existing.ID)
		reply := fmt.Sprintf("Already saved as **%s** — no duplicate created.", existing.FileName)
		if existing.FileName != fileName {
			reply = fmt.Sprintf("This file is already saved as **%s** (uploaded earlier). Skipping duplicate.", existing.FileName)
		}
		s.store.LogActivity(groupID, user.ID, "duplicate_ignored", "/reyna add "+fileName, "duplicate")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"reply":     reply,
			"file_id":   existing.ID,
			"status":    "duplicate",
			"duplicate": true,
		})
		return
	}

	// ── Same filename, different hash → versioned update ──
	// AddFile already auto-bumps version if a row with the same group_id +
	// file_name exists, but we want the *Drive* filename to also reflect the
	// version so users see hii.v2.pdf, not three "hii.pdf" copies.
	versionedName := fileName
	if existingVer := s.store.LatestVersionByName(groupID, fileName); existingVer >= 1 {
		ext := ""
		base := fileName
		if dot := strings.LastIndex(fileName, "."); dot > 0 {
			base = fileName[:dot]
			ext = fileName[dot:]
		}
		versionedName = fmt.Sprintf("%s.v%d%s", base, existingVer+1, ext)
		log.Printf("📝 New version detected for %s — uploading as %s", fileName, versionedName)
	}

	// ── Save file IMMEDIATELY — classify async ──
	// The bot gets an instant response so the checkmark reaction fires
	// within milliseconds. Classification + extraction happen in a background
	// goroutine. The file shows up in the dashboard as "classifying..." and
	// updates to the real subject once Gemini responds.
	if subject == "" {
		subject = "classifying..."
	}

	localID, folderID, _ := s.drive.SmartUpload("", "", user.ID, subject, versionedName, mimeType, fileBytes)
	// Baileys reports the sender from the WhatsApp envelope, so attribution here
	// is authoritative. A capture path that infers the sender instead must set a
	// lower confidence and a method describing how it guessed.
	attrMethod, attrConfidence := model.AttrBaileys, 1.0
	if sharedByName == "" && userPhone == "" {
		attrMethod, attrConfidence = model.AttrNone, 0.0
	}

	dbFile := &model.File{GroupID: groupID, UserID: user.ID, SharedByPhone: userPhone, SharedByName: sharedByName, FileName: versionedName, FileSize: fileSize, MimeType: mimeType, Subject: subject, DriveFileID: localID, DriveFolderID: folderID, Status: "staged", ContentHash: contentHash,
		PostedAt: postedAt, AttributionMethod: attrMethod, AttributionConfidence: attrConfidence}
	saved, err := s.store.AddFile(dbFile)
	if err != nil { log.Printf("❌ DB: %v", err); http.Error(w, `{"error":"db save failed"}`, 500); return }

	s.drive.SaveLocalFileData(saved.ID, fileBytes)
	log.Printf("✅ Saved file %d: %s (%d bytes) — staged (classifying async)", saved.ID, fileName, len(fileBytes))

	total := s.store.CountGroupFiles(groupID)
	v := 1; if saved != nil { v = saved.Version }
	reply := s.reyna.AddResponse(fileName, v, total)
	s.store.LogActivity(groupID, user.ID, "add", "/reyna add "+fileName, "staged")

	// Respond instantly — bot can send checkmark NOW
	json.NewEncoder(w).Encode(map[string]interface{}{"reply": reply, "file_id": saved.ID, "status": "staged"})

	// ── Background: classify + extract ──
	go func(fileID int64, gID int64, fName, mime string, fSize int64, data []byte, grp *model.Group) {
		classifySubject := ""

		// Build candidate folders
		seen := map[string]bool{}
		var existingFolders []string
		for _, sub := range s.store.DistinctSubjectsForGroup(gID) {
			if sub == "" || sub == "classifying..." || seen[sub] { continue }
			seen[sub] = true
			existingFolders = append(existingFolders, sub)
		}
		driveUser := s.store.FindDriveConnectedUser(gID)
		if driveUser != nil && driveUser.GoogleRefresh != "" && driveUser.DriveRootID != "" {
			token, terr := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
			if terr == nil {
				folders, _ := s.drive.ListDriveFolders(token, driveUser.DriveRootID)
				for _, f := range folders {
					n := f["name"]
					if n == "" || seen[n] { continue }
					seen[n] = true
					existingFolders = append(existingFolders, n)
				}
			}
		}

		groupName := ""
		if grp != nil { groupName = grp.Name }
		fmeta := nlp.FileMeta{
			SenderName: sharedByName, SenderPhone: userPhone,
			GroupName: groupName, SharedAt: time.Now(),
		}

		var extractedContent, contentSummary string
		if len(data) > 0 && (strings.Contains(mime, "pdf") || nlp.IsOfficeDoc(mime)) {
			classifySubject, _, _, extractedContent, contentSummary = s.classifier.ClassifyFileWithContent(fName, mime, data, existingFolders, fmeta)
		} else {
			classifySubject, _, _ = s.classifier.ClassifyFile(fName, existingFolders)
		}

		if classifySubject == "" {
			classifySubject = "Uncategorized"
			log.Printf("⚠️  Classification failed for %s — marking as Uncategorized", fName)
			s.store.UpdateFileSubject(fileID, classifySubject)
		} else {
			log.Printf("[NLP] Classified %s → %s (async)", fName, classifySubject)
			s.store.UpdateFileSubject(fileID, classifySubject)
		}

		// Content extraction
		if extractedContent != "" {
			s.store.UpdateFileContent(fileID, extractedContent, contentSummary)
			log.Printf("[EXTRACT] Combined: %s → %s", fName, contentSummary)
		} else {
			content, summary, _ := s.classifier.ExtractContent(fName, mime, fSize, data)
			if content != "" {
				s.store.UpdateFileContent(fileID, content, summary)
				log.Printf("[EXTRACT] Async: %s → %s", fName, summary)
			}
		}
	}(saved.ID, groupID, fileName, mimeType, fileSize, fileBytes, group)
}

func (s *Server) handleBotReaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}

	var req model.ReactionRequest
	json.NewDecoder(r.Body).Decode(&req)

	if req.FileName == "" {
		http.Error(w, `{"error":"file_name required"}`, 400)
		return
	}

	user, _ := s.store.UpsertUser(req.UserPhone, req.UserName)
	group, err := s.store.GetGroupByWAID(req.GroupWAID)
	if err != nil {
		group, _ = s.store.UpsertGroup(req.GroupWAID, "WhatsApp Group", user.ID)
	}
	if group != nil {
		s.store.AddGroupMember(group.ID, user.ID, req.UserPhone, "member")
	}
	groupID := int64(0)
	if group != nil {
		groupID = group.ID
	}

	// Use NLP to classify the file into a folder
	var existingFolders []string
	driveUser := s.store.FindDriveConnectedUser(groupID)
	if driveUser != nil && driveUser.GoogleRefresh != "" && driveUser.DriveRootID != "" {
		token, terr := s.drive.GetValidToken(driveUser.GoogleToken, driveUser.GoogleRefresh, 0)
		if terr == nil {
			folders, _ := s.drive.ListDriveFolders(token, driveUser.DriveRootID)
			for _, f := range folders {
				existingFolders = append(existingFolders, f["name"])
			}
		}
	}

	subject, _, _ := s.classifier.ClassifyFile(req.FileName, existingFolders)
	log.Printf("[REACTION] File=%s classified as=%s by NLP", req.FileName, subject)

	file := &model.File{
		GroupID:       groupID,
		UserID:        user.ID,
		SharedByPhone: req.UserPhone,
		SharedByName:  req.UserName,
		FileName:      req.FileName,
		FileSize:      req.FileSize,
		MimeType:      req.MimeType,
		Subject:       subject,
		WAMessageID:   req.WAMessageID,
		Status:        "staged",
	}
	saved, _ := s.store.AddFile(file)
	s.store.LogActivity(groupID, user.ID, "reaction_stage", req.FileName, "staged via "+req.Emoji)

	reply := s.reyna.ReactionStagedResponse(req.FileName)
	json.NewEncoder(w).Encode(map[string]interface{}{"reply": reply, "file_id": saved.ID, "status": "staged", "subject": subject})
}

func (s *Server) handleBotSyncGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { http.Error(w, `{"error":"method not allowed"}`, 405); return }
	var req struct {
		WAID        string `json:"wa_id"`
		Name        string `json:"name"`
		MemberCount int    `json:"member_count"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.WAID == "" { http.Error(w, `{"error":"wa_id required"}`, 400); return }
	// Don't default to "WhatsApp Group" — if the bot couldn't get the name,
	// keep whatever name the DB already has. Only update if we got a real one.

	// Try to get existing group first, create if not exists
	group, err := s.store.GetGroupByWAID(req.WAID)
	if err != nil || group == nil {
		// Insert directly without foreign key on created_by
		s.store.InsertGroupRaw(req.WAID, req.Name, req.MemberCount)
		group, _ = s.store.GetGroupByWAID(req.WAID)
	}

	if group != nil {
		// Only update name if we got a real one — never overwrite with empty or placeholder
		if req.Name != "" && req.Name != "WhatsApp Group" && req.Name != group.Name {
			s.store.UpdateGroupName(group.ID, req.Name)
			log.Printf("[SYNC-GROUP] Updated group %d name: %q → %q", group.ID, group.Name, req.Name)
		}
		if req.MemberCount > 0 {
			s.store.UpdateGroupMemberCount(group.ID, req.MemberCount)
		}
		// Create group settings with defaults if they don't exist yet.
		// Do NOT re-enable if the user explicitly disabled the group —
		// that was overriding the dashboard toggle on every bot sync.
		gs := s.store.GetGroupSettings(group.ID)
		if gs.GroupID == 0 || !s.store.GroupSettingsExist(group.ID) {
			gs.GroupID = group.ID
			gs.Enabled = true
			s.store.UpsertGroupSettings(gs)
			log.Printf("Auto-enabled group %d (%s) via first sync", group.ID, req.Name)
		}
	}

	gid := int64(0)
	if group != nil { gid = group.ID }
	json.NewEncoder(w).Encode(map[string]interface{}{"synced": true, "group_id": gid})
}

func (s *Server) handleEnabledGroups(w http.ResponseWriter, r *http.Request) {
	ids := s.store.GetAllEnabledGroupWAIDs()
	if ids == nil {
		ids = []string{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": ids})
}

func (s *Server) handleGroupStates(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.store.GetAllGroups()
	states := make(map[string]map[string]interface{})
	for _, g := range groups {
		gs := s.store.GetGroupSettings(g.ID)
		mode := gs.TrackingMode
		if mode == "" {
			mode = "auto"
		}
		emoji := gs.ReactionEmoji
		if emoji == "" {
			emoji = "📌"
		}
		states[g.WAID] = map[string]interface{}{
			"enabled":        gs.Enabled,
			"hidden":         gs.Hidden,
			"tracking_mode":  mode,
			"reaction_emoji": emoji,
		}
	}
	json.NewEncoder(w).Encode(states)
}

func (s *Server) handleKnownGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.store.GetAllGroups()
	var ids []string
	for _, g := range groups {
		ids = append(ids, g.WAID)
	}
	if ids == nil { ids = []string{} }
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": ids})
}

func (s *Server) handleGroupMode(w http.ResponseWriter, r *http.Request) {
	waID := r.URL.Query().Get("wa_id")
	if waID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"mode": "auto", "enabled": false})
		return
	}
	enabled := s.store.IsGroupEnabled(waID)
	mode := s.store.GetGroupTrackingMode(waID)
	json.NewEncoder(w).Encode(map[string]interface{}{"mode": mode, "enabled": enabled})
}
