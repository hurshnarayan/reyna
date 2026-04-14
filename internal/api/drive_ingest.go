package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/auth"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/search"
)

// handleDriveIngest starts an async Drive → Reyna sync and returns
// immediately with a job handle. The actual work runs in a goroutine and
// reports progress via the in-memory job registry. The frontend polls
// /api/drive/ingest/status to drive a sticky progress pill and lets the
// user navigate the dashboard without losing the sync.
//
// Per-file cost: one Drive download + one Gemini doc extract + one Gemini
// embedding. Rate-limited so the free tier stays under throttle limits.
// Hard caps: 200 files per invocation, 20 MB per file, 500 folders walked.
//
// Ingested files land in a dedicated synthetic "My Drive" group keyed by
// `personal:<phone>` so Recall can scope to them cleanly and repeat runs
// are idempotent (dedupes by drive_file_id).
func (s *Server) handleDriveIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	user, err := s.store.GetUserByID(userID)
	if err != nil || user == nil {
		http.Error(w, `{"error":"user not found"}`, 404)
		return
	}
	if user.GoogleRefresh == "" || user.DriveRootID == "" {
		http.Error(w, `{"error":"google drive not connected"}`, 400)
		return
	}

	// If a sync is already running for this user, return the existing job
	// rather than starting a duplicate goroutine. Frontend will happily
	// keep polling whichever job id comes back.
	job, fresh := s.jobs.start(userID, JobKindIngestDrive)
	if !fresh {
		json.NewEncoder(w).Encode(job)
		return
	}

	// Kick off the worker. Handler returns the freshly-registered job so
	// the frontend can show a progress pill straight away without waiting.
	go s.runDriveIngest(user.ID, user.Phone, user.Name, user.Email, user.GoogleToken, user.GoogleRefresh, user.DriveRootID)
	json.NewEncoder(w).Encode(job)
}

// handleDriveIngestStatus reports the current job state (progress, counts,
// final verdict). Returns 404 when no sync has ever been run — the UI uses
// that to hide the pill entirely.
func (s *Server) handleDriveIngestStatus(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	job := s.jobs.get(userID, JobKindIngestDrive)
	if job == nil {
		w.WriteHeader(204) // no content — no job ever run
		return
	}
	json.NewEncoder(w).Encode(job)
}

// runDriveIngest is the goroutine body. It's deliberately tolerant of
// failures — each file is independent, and a Gemini/Drive blip on one
// doesn't abort the whole sweep.
func (s *Server) runDriveIngest(userID int64, phone, name, email, token, refresh, driveRootID string) {
	// Refresh token once at start; skip the error path on refresh failure
	// since the ingest goroutine can still use the existing token for a
	// while. Persist whichever token we end up using.
	tok, err := s.drive.GetValidToken(token, refresh, 0)
	if err != nil || tok == "" {
		tok = token
	}
	if tok != token {
		_ = s.store.UpdateUserGoogle(userID, email, tok, refresh, driveRootID)
	}

	personalGID, err := s.store.EnsureUserPersonalGroup(userID, phone)
	if err != nil {
		s.jobs.finish(userID, JobKindIngestDrive, JobStateFailed, "could not create My Drive group: "+err.Error())
		return
	}

	type discovered struct{ ID, Name, MimeType, Size, ParentID, ParentName string }
	var queue []discovered
	type qItem struct{ id, name string }
	folderQueue := []qItem{{id: driveRootID, name: ""}}
	seen := map[string]bool{}
	walkStart := time.Now()
	for len(folderQueue) > 0 && len(queue) < 500 {
		curr := folderQueue[0]
		folderQueue = folderQueue[1:]
		if seen[curr.id] {
			continue
		}
		seen[curr.id] = true
		files, _ := s.drive.ListDriveFiles(tok, curr.id)
		for _, f := range files {
			id, _ := f["id"].(string)
			fn, _ := f["name"].(string)
			mime, _ := f["mime_type"].(string)
			sz, _ := f["size"].(string)
			queue = append(queue, discovered{
				ID: id, Name: fn, MimeType: mime, Size: sz,
				ParentID: curr.id, ParentName: curr.name,
			})
		}
		subs, _ := s.drive.ListDriveFolders(tok, curr.id)
		for _, sub := range subs {
			folderQueue = append(folderQueue, qItem{id: sub["id"], name: sub["name"]})
		}
	}
	log.Printf("[INGEST] discovered %d files under drive root in %s", len(queue), time.Since(walkStart).Round(time.Millisecond))

	// Seed totals so the progress bar has a denominator.
	total := len(queue)
	if total > 200 {
		total = 200
	}
	s.jobs.update(userID, JobKindIngestDrive, func(j *Job) {
		j.Total = total
		j.Message = "walking your drive..."
	})

	indexed, skipped, errored, extractedCount := 0, 0, 0, 0
	start := time.Now()

	for i, d := range queue {
		if i >= 200 {
			break
		}
		s.jobs.update(userID, JobKindIngestDrive, func(j *Job) {
			j.Done = i
			j.Message = "ingesting " + d.Name
		})
		if d.ID == "" || d.Name == "" {
			skipped++
			continue
		}
		if strings.HasPrefix(d.MimeType, "application/vnd.google-apps") {
			skipped++
			continue
		}
		if s.store.FileExistsByDriveID(d.ID) {
			skipped++
			continue
		}
		size := int64(0)
		if d.Size != "" {
			size, _ = strconv.ParseInt(d.Size, 10, 64)
		}
		if size > 20*1024*1024 {
			log.Printf("[INGEST] skipping %s — too large (%d bytes)", d.Name, size)
			skipped++
			continue
		}
		subject := d.ParentName
		if subject == "" {
			subject = "My Drive"
		}

		file := &model.File{
			GroupID: personalGID, UserID: userID,
			SharedByPhone: phone, SharedByName: name,
			FileName: d.Name, FileSize: size, MimeType: d.MimeType,
			DriveFileID: d.ID, DriveFolderID: d.ParentID,
			Subject: subject, Status: "committed",
		}
		saved, err := s.store.AddFile(file)
		if err != nil || saved == nil {
			errored++
			log.Printf("[INGEST] AddFile failed for %s: %v", d.Name, err)
			continue
		}
		data, derr := s.drive.DownloadFromDrive(tok, d.ID)
		if derr != nil || len(data) == 0 {
			log.Printf("[INGEST] download failed for %s: %v", d.Name, derr)
			indexed++
			continue
		}
		content, summary := s.classifier.ExtractContent(d.Name, d.MimeType, size, data)
		if content != "" {
			_ = s.store.UpdateFileContent(saved.ID, content, summary)
			extractedCount++
		}
		if s.search.IsEnabled() && content != "" {
			meta := search.FileMetadata{
				FileID: saved.ID, UserID: saved.UserID, GroupID: saved.GroupID,
				FileName: saved.FileName, Subject: saved.Subject,
				SenderName: saved.SharedByName, GroupName: "My Drive",
				SharedAt: saved.CreatedAt, Summary: summary,
			}
			if err := s.search.IndexFile(meta, content); err != nil {
				log.Printf("[INGEST] qdrant upsert failed for %s: %v", d.Name, err)
			}
		}
		_ = s.drive.SaveLocalFileData(saved.ID, data)

		indexed++
		// Errored, indexed, skipped countered into job state so the UI
		// can render a live (done/total) strip without extra fetches.
		s.jobs.update(userID, JobKindIngestDrive, func(j *Job) {
			j.Done = i + 1
			j.Errored = errored
			j.Skipped = skipped
		})
		log.Printf("[INGEST] %d/%d %s (%s, %d bytes, extracted=%v)", i+1, total, d.Name, subject, len(data), content != "")
		time.Sleep(350 * time.Millisecond)
	}
	dur := time.Since(start).Round(time.Second)
	log.Printf("[INGEST] user=%d indexed=%d extracted=%d skipped=%d errored=%d total=%d in %s",
		userID, indexed, extractedCount, skipped, errored, len(queue), dur)

	s.jobs.update(userID, JobKindIngestDrive, func(j *Job) {
		j.Done = total
		j.Errored = errored
		j.Skipped = skipped
	})
	msg := "ingested " + strconv.Itoa(indexed) + " files, " + strconv.Itoa(extractedCount) + " with content"
	if indexed == 0 && skipped > 0 {
		msg = "nothing new, " + strconv.Itoa(skipped) + " already known"
	}
	s.jobs.finish(userID, JobKindIngestDrive, JobStateDone, msg)
}
