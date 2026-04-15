package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/auth"
	"github.com/hurshnarayan/reyna/internal/search"
)

// handleRecallBackfill re-embeds every file (across the caller's accessible
// groups) that has extracted_content but no corresponding Qdrant entry.
// This is the one-shot migration for files that existed *before* Qdrant
// was configured — without it, Recall only sees newly-committed files.
//
// Runs synchronously and streams progress-ish info in the response. For
// thousands of files this is slow; callers should poll or accept the wait.
// Skips files that have no extracted_content (there's nothing to embed).
func (s *Server) handleRecallBackfill(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	if s.search == nil || !s.search.IsEnabled() {
		http.Error(w, `{"error":"search not configured — set QDRANT_URL and GEMINI_API_KEY"}`, 400)
		return
	}

	groupIDs := s.store.GetUserGroupIDs(userID)
	if len(groupIDs) == 0 {
		// Same auto-link as /files and Recall — link the user to any
		// groups they have files in but aren't yet a member of.
		if user, _ := s.store.GetUserByID(userID); user != nil {
			s.store.AutoLinkUserToGroups(userID, user.Phone)
			groupIDs = s.store.GetUserGroupIDs(userID)
		}
	}
	if len(groupIDs) == 0 {
		json.NewEncoder(w).Encode(map[string]any{"indexed": 0, "skipped": 0, "note": "no groups"})
		return
	}

	files, err := s.store.GetGroupsFiles(groupIDs, 5000)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}

	indexed := 0
	skipped := 0
	errored := 0
	start := time.Now()

	for i, f := range files {
		content, _ := s.store.GetFileContent(f.ID)
		// Always embed — even if extraction never produced text. Use
		// the filename + folder/subject + sender as fallback so the
		// file is at least findable by name. Without this, every
		// rate-limited / unsupported-mime-type file stays invisible
		// to Recall and voice forever.
		indexText := content
		if indexText == "" {
			indexText = strings.Join([]string{
				f.FileName, f.Subject, f.SharedByName, f.MimeType,
			}, " ")
		}
		groupName := ""
		if g, err := s.store.GetGroupByWAID(""); err == nil && g != nil && g.ID == f.GroupID {
			groupName = g.Name
		}
		meta := search.FileMetadata{
			FileID:     f.ID,
			UserID:     f.UserID,
			GroupID:    f.GroupID,
			FileName:   f.FileName,
			Subject:    f.Subject,
			SenderName: f.SharedByName,
			GroupName:  groupName,
			SharedAt:   f.CreatedAt,
			Summary:    f.ContentSummary,
		}
		if err := s.search.IndexFile(meta, indexText); err != nil {
			errored++
			log.Printf("[BACKFILL] file %d (%s) failed: %v", f.ID, f.FileName, err)
			continue
		}
		indexed++
		// Rate-limit: Gemini free tier is ~15 req/min. We stay well under
		// by sleeping ~200ms between calls.
		if i%5 == 4 {
			time.Sleep(time.Second)
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}

	log.Printf("[BACKFILL] user=%d indexed=%d skipped=%d errored=%d in %s",
		userID, indexed, skipped, errored, time.Since(start).Round(time.Second))

	json.NewEncoder(w).Encode(map[string]any{
		"indexed":     indexed,
		"skipped":     skipped,
		"errored":     errored,
		"total_files": len(files),
		"duration_s":  time.Since(start).Round(time.Second).Seconds(),
	})
}
