package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/hurshnarayan/reyna/internal/auth"
	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/nlp"
	"github.com/hurshnarayan/reyna/internal/search"
)

// ── Reyna's Memory ──
// Persistent user-level context. Each user can add multiple named "memories"
// (their syllabus, exam schedule, study style, etc.), toggle them on/off,
// and delete them. Active memories inform Recall answers.

// handleMemoryList: GET /api/memory  — list all memories for the logged-in user.
func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	memories, err := s.store.ListMemories(userID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"memories": memories})
}

// handleMemoryCreate: POST /api/memory
// Body: { title, content, source?, always_include? }
func (s *Server) handleMemoryCreate(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	var req struct {
		Title          string `json:"title"`
		Content        string `json:"content"`
		Source         string `json:"source"`
		SourceFileName string `json:"source_file_name"`
		AlwaysInclude  bool   `json:"always_include"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	if req.Title == "" || req.Content == "" {
		http.Error(w, `{"error":"title and content are required"}`, 400)
		return
	}
	m := &model.UserMemory{
		UserID: userID, Title: req.Title, Content: req.Content,
		Source: req.Source, SourceFileName: req.SourceFileName,
		IsActive: true, AlwaysInclude: req.AlwaysInclude,
	}
	created, err := s.store.CreateMemory(m)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	// Index to Qdrant asynchronously so the UI doesn't block on embedding.
	go s.indexMemoryAsync(created)
	json.NewEncoder(w).Encode(map[string]any{"memory": created})
}

// handleMemoryUpdate: POST /api/memory/update  — body: full memory object.
func (s *Server) handleMemoryUpdate(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	var req struct {
		ID            int64  `json:"id"`
		Title         string `json:"title"`
		Content       string `json:"content"`
		IsActive      bool   `json:"is_active"`
		AlwaysInclude bool   `json:"always_include"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}
	if req.ID == 0 {
		http.Error(w, `{"error":"id required"}`, 400)
		return
	}
	m := &model.UserMemory{
		ID: req.ID, UserID: userID, Title: req.Title, Content: req.Content,
		IsActive: req.IsActive, AlwaysInclude: req.AlwaysInclude,
	}
	if err := s.store.UpdateMemory(m); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	updated, _ := s.store.GetMemory(req.ID, userID)
	if updated != nil {
		go s.indexMemoryAsync(updated)
	}
	json.NewEncoder(w).Encode(map[string]any{"memory": updated})
}

// handleMemoryToggle: POST /api/memory/toggle  — body: { id, is_active }.
func (s *Server) handleMemoryToggle(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	var req struct {
		ID       int64 `json:"id"`
		IsActive bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}
	if err := s.store.ToggleMemory(req.ID, userID, req.IsActive); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "is_active": req.IsActive})
}

// handleMemoryDelete: POST /api/memory/delete  — body: { id }.
func (s *Server) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	userID := auth.GetUserID(r)
	if userID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}
	if err := s.store.DeleteMemory(req.ID, userID); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	// Purge from Qdrant. Non-fatal if it fails — the is_active gate in the
	// store would still filter it out.
	if s.search != nil {
		if err := s.search.DeleteMemory(req.ID); err != nil {
			log.Printf("[MEMORY] delete embedding %d: %v", req.ID, err)
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// collectMemorySources builds the pseudo-sources that Reyna's Memory
// contributes to a Recall/QA answer for a given user and query.
// Combines two kinds of memories:
//   - always_include: returned in full, regardless of query — small pinned
//     facts like "my finals start May 10" that should shape every answer.
//   - semantically-relevant chunks (Qdrant): parts of bigger memories (e.g.
//     the syllabus) that match the question's meaning.
//
// Deactivated memories (is_active=0) are excluded. Inactive Qdrant points
// are also filtered at DB-check time so a recently-toggled memory can't leak
// in before the vector store catches up.
func (s *Server) collectMemorySources(userID int64, query string) []nlp.QASource {
	out := []nlp.QASource{}

	// 1) Always-include memories (no embedding needed).
	if pinned, err := s.store.ActiveAlwaysIncludeMemories(userID); err == nil {
		for _, m := range pinned {
			out = append(out, buildMemorySource(m.Title, m.Content))
		}
	}

	// 2) Semantic memory recall (if enabled).
	if s.search != nil && s.search.IsEnabled() && strings.TrimSpace(query) != "" {
		if hits, err := s.search.SearchMemories(query, userID, 4); err == nil {
			seen := map[int64]bool{}
			for _, h := range hits {
				// Gate on is_active so a toggled-off memory can't slip in.
				if !s.store.MemoryIsActive(h.MemoryID) {
					continue
				}
				// De-dupe: a memory might already be pinned via always_include.
				if seen[h.MemoryID] {
					continue
				}
				seen[h.MemoryID] = true
				// Score floor: below ~0.35 cosine similarity, hits are noise.
				if h.Score < 0.35 {
					continue
				}
				out = append(out, buildMemorySource(h.Title, h.ChunkText))
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	// Defensive cap — never let memory alone exceed 4 pseudo-sources.
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func buildMemorySource(title, content string) nlp.QASource {
	return nlp.QASource{
		FileName:   "[Memory] " + title,
		Content:    content,
		SenderName: "You (saved memory)",
		Subject:    "Reyna's Memory",
	}
}

// indexMemoryAsync re-embeds a memory's content. Small memories marked
// always_include skip embedding entirely — they're already cheap to include
// in every prompt directly, and re-embedding on every toggle is wasteful.
func (s *Server) indexMemoryAsync(m *model.UserMemory) {
	if s.search == nil || !s.search.IsEnabled() || m == nil {
		return
	}
	if !m.IsActive {
		// Toggled off — keep the DB row but remove from semantic search.
		if err := s.search.DeleteMemory(m.ID); err != nil {
			log.Printf("[MEMORY] remove embedding %d: %v", m.ID, err)
		}
		return
	}
	meta := search.MemoryMetadata{MemoryID: m.ID, UserID: m.UserID, Title: m.Title}
	if err := s.search.IndexMemory(meta, m.Content); err != nil {
		log.Printf("[MEMORY] index %d failed: %v", m.ID, err)
	} else {
		log.Printf("[MEMORY] indexed #%d %q", m.ID, m.Title)
	}
}
