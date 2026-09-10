// Package attribution decides who shared a file and when.
//
// On-device this is the hard problem and the whole product. A file sitting in
// WhatsApp's media folder carries no sender: the name, the chat and the time
// have to be reconstructed by matching it against messages learned about
// separately, from a notification the phone saw or a line in a chat export.
// That match can fail and it can be ambiguous, so every result carries a
// method and a confidence, and callers are forbidden from naming anyone below
// the threshold.
//
// The rule that matters: never invent a sender. "Shared in Sem 5 CS, eleven
// weeks ago" is a good answer. "Shared by Mohit" when we are guessing is not,
// because the user cannot tell the difference and will act on it.
//
// This is the server-side twin of the Kotlin implementation in the Android app
// (app/src/main/java/app/reyna/attribution/Attribution.kt). The two are kept
// behaviourally identical: the app attributes at capture time so it can show a
// result immediately, and the server re-attributes when new events arrive from
// any device. If you change the rules in one, change them in both.
package attribution

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/model"
)

// Event is a message we know about, from a notification or a chat export.
//
// Stored whether or not a file ever turns up for it: a file downloaded hours
// later still needs the event to exist, which is why the join runs in both
// directions.
type Event struct {
	ID             int64
	ChatKey        string
	ChatName       string
	SenderDisplay  string
	PostedAt       time.Time
	Text           string
	AttachmentName string
	HasAttachment  bool
	Source         string
}

// File is a captured file as the join sees it.
type File struct {
	ID       int64
	DiskName string
	// MTime is when the file reached the disk, which is not when it was sent.
	MTime  time.Time
	IsSent bool
}

// Link is one candidate match, with why and how strongly.
type Link struct {
	FileID     int64
	EventID    int64
	Method     string
	Confidence float64
}

// Result is the outcome for one file.
//
// Candidates holds every match considered, not only the winner, so a chat
// export imported later can promote a better one without the earlier reasoning
// being lost, and a wrong guess can be explained after the fact.
type Result struct {
	FileID     int64
	Best       *Link
	Candidates []Link
}

func (r Result) Confidence() float64 {
	if r.Best == nil {
		return 0
	}
	return r.Best.Confidence
}

func (r Result) Method() string {
	if r.Best == nil {
		return model.AttrNone
	}
	return r.Best.Method
}

// CanName reports whether the sender may be stated.
func (r Result) CanName() bool { return r.Confidence() >= model.AttributionMinNamed }

const (
	sixHours   = 6 * time.Hour
	twoHours   = 2 * time.Hour
	tenMinutes = 10 * time.Minute
)

// waName matches WhatsApp-generated media names: DOC-20260818-WA0007.pdf.
//
// These carry two signals worth having when nothing else matches. The date is
// exact, and the counter is sequential, so WA0007 arrived after WA0003 that
// day, which orders files within a day with no other source at all.
//
// Whether documents keep their original name or get renamed to this form is
// the single highest-value unknown in the capture design: it decides whether
// the exact rule or the date rule is the common path.
var waName = regexp.MustCompile(`^[A-Za-z]{3}-(\d{4})(\d{2})(\d{2})-WA(\d{4})\.`)

// WANameDate returns the date embedded in a WhatsApp-generated filename.
func WANameDate(diskName string) (year, month, day int, ok bool) {
	m := waName.FindStringSubmatch(diskName)
	if m == nil {
		return 0, 0, 0, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	return y, mo, d, true
}

// WANameCounter returns the intra-day sequence number, which orders files
// within a day even when nothing else is known.
func WANameCounter(diskName string) (int, bool) {
	m := waName.FindStringSubmatch(diskName)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[4])
	return n, err == nil
}

func isImageFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".webp")
}

// Attribute matches one file against the events we know about.
//
// The chain runs strongest first and collects every candidate rather than
// stopping at the first hit, because a later export can promote a weaker link
// and the alternatives should still be on record when it does.
//
// zone interprets the date embedded in a WhatsApp filename. WhatsApp stamps it
// with the phone's local date, so local is correct, but it has to be explicit:
// left implicit it silently mis-dates every file near midnight for anyone whose
// zone differs from wherever the comparison runs, and the symptom is a lost
// attribution rather than an error.
func Attribute(f File, events []Event, zone *time.Location) Result {
	if zone == nil {
		zone = time.Local
	}

	// The user shared it themselves. Certain, and free.
	if f.IsSent {
		link := Link{FileID: f.ID, Method: model.AttrSelfSent, Confidence: 1.0}
		return Result{FileID: f.ID, Best: &link, Candidates: []Link{link}}
	}

	diskLower := strings.ToLower(f.DiskName)
	fy, fm, fd, hasDate := WANameDate(f.DiskName)

	var candidates []Link
	for _, e := range events {
		if strings.EqualFold(e.ChatName, "WhatsApp") || strings.EqualFold(e.SenderDisplay, "WhatsApp") {
			continue
		}
		if strings.TrimSpace(e.ChatName) == "" && strings.TrimSpace(e.SenderDisplay) == "" {
			continue
		}

		dt := e.PostedAt.Sub(f.MTime)
		if dt < 0 {
			dt = -dt
		}

		// The event names this exact file. Nothing beats it.
		if e.AttachmentName != "" && strings.EqualFold(e.AttachmentName, f.DiskName) {
			conf := 0.85
			if dt <= sixHours {
				conf = 0.95
			}
			candidates = append(candidates, Link{f.ID, e.ID, model.AttrExport, conf})
			continue
		}

		// The filename appears inside the message text.
		if diskLower != "" && strings.Contains(strings.ToLower(e.Text), diskLower) && dt <= twoHours {
			candidates = append(candidates, Link{f.ID, e.ID, model.AttrNotification, 0.75})
			continue
		}

		// High-confidence temporal notification match for unnamed media attachments (e.g. photos)
		if isImageFile(f.DiskName) && e.HasAttachment && e.AttachmentName == "" && dt <= 5*time.Minute {
			candidates = append(candidates, Link{f.ID, e.ID, model.AttrNotification, 0.85})
			continue
		}

		// The name is WhatsApp-generated, so it gives the date but not which
		// message. Resolved below once we know how many share that day.
		if hasDate && e.HasAttachment && sameDay(e.PostedAt, fy, fm, fd, zone) {
			candidates = append(candidates, Link{f.ID, e.ID, model.AttrDateUnique, 0.70})
			continue
		}

		// Nothing but proximity. Weak by construction, and only ever enough to
		// say "shared in this chat", never a name.
		if e.HasAttachment && dt <= tenMinutes {
			candidates = append(candidates, Link{f.ID, e.ID, model.AttrTimeOnly, 0.30})
		}
	}

	// A date-unique claim is only worth naming when the day really is unique.
	// Several candidates on the same day means we are picking one, which is a
	// guess, so it drops below the threshold and the nearest in time wins.
	dateHits := 0
	for _, c := range candidates {
		if c.Method == model.AttrDateUnique {
			dateHits++
		}
	}
	if dateHits > 1 {
		nearest := -1
		var bestGap time.Duration
		for i, c := range candidates {
			if c.Method != model.AttrDateUnique {
				continue
			}
			e := findEvent(events, c.EventID)
			if e == nil {
				continue
			}
			gap := e.PostedAt.Sub(f.MTime)
			if gap < 0 {
				gap = -gap
			}
			if nearest < 0 || gap < bestGap {
				nearest, bestGap = i, gap
			}
		}
		for i := range candidates {
			if candidates[i].Method != model.AttrDateUnique {
				continue
			}
			candidates[i].Method = model.AttrTimeWindow
			if i == nearest {
				candidates[i].Confidence = 0.45
			} else {
				candidates[i].Confidence = 0.30
			}
		}
	}

	var best *Link
	var bestEvent *Event
	var bestGap time.Duration
	for i := range candidates {
		c := &candidates[i]
		e := findEvent(events, c.EventID)
		gap := time.Duration(1<<63 - 1)
		if e != nil {
			gap = e.PostedAt.Sub(f.MTime)
			if gap < 0 {
				gap = -gap
			}
		}

		better := false
		if best == nil {
			better = true
		} else if c.Confidence > best.Confidence {
			better = true
		} else if c.Confidence == best.Confidence {
			cHasSender := e != nil && strings.TrimSpace(e.SenderDisplay) != ""
			bestHasSender := bestEvent != nil && strings.TrimSpace(bestEvent.SenderDisplay) != ""
			if cHasSender && !bestHasSender {
				better = true
			} else if cHasSender == bestHasSender && gap < bestGap {
				better = true
			}
		}

		if better {
			best = c
			bestEvent = e
			bestGap = gap
		}
	}
	return Result{FileID: f.ID, Best: best, Candidates: candidates}
}

func findEvent(events []Event, id int64) *Event {
	for i := range events {
		if events[i].ID == id {
			return &events[i]
		}
	}
	return nil
}

func sameDay(t time.Time, y, m, d int, zone *time.Location) bool {
	lt := t.In(zone)
	return lt.Year() == y && int(lt.Month()) == m && lt.Day() == d
}

// Describe renders what Reyna is allowed to say about a file.
//
// This is where honest degradation is enforced. Three bands, and the name only
// survives the top one. Mirrors model.File.SenderKnown and the Kotlin
// Attribution.describe so every surface says the same thing.
func Describe(confidence float64, sender, chat, when string) string {
	switch {
	case confidence >= model.AttributionMinNamed && sender != "":
		return sender + " · " + when
	case confidence >= 0.30 && chat != "":
		return chat + " · " + when
	default:
		return "Found on your phone · " + when
	}
}
