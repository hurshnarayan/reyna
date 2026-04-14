// Package search wires Qdrant + Gemini embeddings into a simple high-level
// API for Reyna's Recall (semantic search over files) and Memory (persistent
// user context). This is the only package that knows about both the vector
// DB and the embedding model — everything else calls through Service.
package search

import (
	"log"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/integrations/llm"
	"github.com/hurshnarayan/reyna/internal/integrations/qdrant"
)

type Service struct {
	qd       *qdrant.Client
	embedKey string // Gemini API key for embeddings
}

// New returns a search service. If qdrantURL or geminiKey is empty, the
// service is disabled and all methods become no-ops / fall back gracefully.
func New(qdrantURL, qdrantAPIKey, geminiKey string) *Service {
	client := qdrant.New(qdrantURL, qdrantAPIKey)
	s := &Service{qd: client, embedKey: geminiKey}
	if s.IsEnabled() {
		// Best-effort collection setup at boot. Failures are logged, not fatal.
		if err := client.EnsureCollection(qdrant.CollectionFiles, llm.GeminiEmbedDim); err != nil {
			log.Printf("[Search] EnsureCollection %s failed: %v", qdrant.CollectionFiles, err)
		}
		if err := client.EnsureCollection(qdrant.CollectionMemories, llm.GeminiEmbedDim); err != nil {
			log.Printf("[Search] EnsureCollection %s failed: %v", qdrant.CollectionMemories, err)
		}
	}
	return s
}

// IsEnabled reports whether semantic search is actually usable.
func (s *Service) IsEnabled() bool {
	return s != nil && s.qd.IsEnabled() && s.embedKey != ""
}

// FileMetadata is the payload we store alongside each file vector so we can
// surface WHO/WHEN/WHERE context without a DB lookup during Recall.
type FileMetadata struct {
	FileID     int64
	UserID     int64
	GroupID    int64
	FileName   string
	Subject    string
	SenderName string
	GroupName  string
	SharedAt   time.Time
	Summary    string
}

// IndexFile embeds file content and stores it in Qdrant. Called asynchronously
// after a file's extracted_content is available. Combines summary + content
// so both short-form matches (summary) and long-form matches (body) work.
func (s *Service) IndexFile(meta FileMetadata, content string) error {
	if !s.IsEnabled() {
		return nil
	}
	text := strings.TrimSpace(meta.FileName + ". " + meta.Subject + ". " + meta.Summary + "\n" + content)
	if text == "" {
		return nil
	}
	vec, err := llm.EmbedText(s.embedKey, text)
	if err != nil {
		return err
	}
	point := qdrant.Point{
		ID:     qdrant.FilePointID(meta.FileID),
		Vector: vec,
		Payload: map[string]any{
			"file_id":     meta.FileID,
			"user_id":     meta.UserID,
			"group_id":    meta.GroupID,
			"file_name":   meta.FileName,
			"subject":     meta.Subject,
			"sender_name": meta.SenderName,
			"group_name":  meta.GroupName,
			"shared_at":   meta.SharedAt.Format(time.RFC3339),
			"summary":     meta.Summary,
		},
	}
	return s.qd.Upsert(qdrant.CollectionFiles, []qdrant.Point{point})
}

// SearchFiles returns file IDs and scores ranked by semantic similarity to the
// query, restricted to the given group IDs (a user's accessible groups).
// When groupIDs is nil the search is unfiltered — caller is responsible for
// auth. Returns nil without error when disabled, so callers can fall back.
func (s *Service) SearchFiles(query string, groupIDs []int64, limit int) ([]FileHit, error) {
	if !s.IsEnabled() || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	vec, err := llm.EmbedText(s.embedKey, query)
	if err != nil {
		return nil, err
	}
	var filter map[string]any
	if len(groupIDs) > 0 {
		any := make([]any, len(groupIDs))
		for i, g := range groupIDs {
			any[i] = g
		}
		filter = qdrant.MustMatchAny("group_id", any)
	}
	results, err := s.qd.Search(qdrant.CollectionFiles, vec, limit, filter)
	if err != nil {
		return nil, err
	}
	hits := make([]FileHit, 0, len(results))
	for _, r := range results {
		h := FileHit{Score: r.Score}
		if v, ok := r.Payload["file_id"].(float64); ok {
			h.FileID = int64(v)
		}
		if v, ok := r.Payload["file_name"].(string); ok {
			h.FileName = v
		}
		if v, ok := r.Payload["subject"].(string); ok {
			h.Subject = v
		}
		if v, ok := r.Payload["summary"].(string); ok {
			h.Summary = v
		}
		if v, ok := r.Payload["sender_name"].(string); ok {
			h.SenderName = v
		}
		if v, ok := r.Payload["group_name"].(string); ok {
			h.GroupName = v
		}
		hits = append(hits, h)
	}
	return hits, nil
}

type FileHit struct {
	FileID     int64   `json:"file_id"`
	FileName   string  `json:"file_name"`
	Subject    string  `json:"subject"`
	Summary    string  `json:"summary"`
	SenderName string  `json:"sender_name"`
	GroupName  string  `json:"group_name"`
	Score      float64 `json:"score"`
}

// DeleteFile removes a file's vector from Qdrant.
func (s *Service) DeleteFile(fileID int64) error {
	if !s.IsEnabled() {
		return nil
	}
	return s.qd.Delete(qdrant.CollectionFiles, []any{qdrant.FilePointID(fileID)})
}

// ── Memory (Reyna's Memory) ──

type MemoryMetadata struct {
	MemoryID int64
	UserID   int64
	Title    string
}

// IndexMemory embeds memory content (chunked if large) and stores it.
// Replaces any existing chunks for the memory by deleting via filter first.
func (s *Service) IndexMemory(meta MemoryMetadata, content string) error {
	if !s.IsEnabled() {
		return nil
	}
	// Remove any existing chunks for this memory first — cheap and avoids
	// orphaned chunks when content shrinks.
	if err := s.qd.DeleteByFilter(qdrant.CollectionMemories, qdrant.MustMatch("memory_id", meta.MemoryID)); err != nil {
		log.Printf("[Search] clean old memory chunks: %v", err)
	}
	chunks := llm.ChunkText(content, 1500)
	points := make([]qdrant.Point, 0, len(chunks))
	for i, chunk := range chunks {
		vec, err := llm.EmbedText(s.embedKey, chunk)
		if err != nil {
			return err
		}
		points = append(points, qdrant.Point{
			ID:     qdrant.MemoryPointID(meta.MemoryID, i),
			Vector: vec,
			Payload: map[string]any{
				"memory_id":   meta.MemoryID,
				"user_id":     meta.UserID,
				"title":       meta.Title,
				"chunk_index": i,
				"chunk_text":  chunk,
			},
		})
	}
	return s.qd.Upsert(qdrant.CollectionMemories, points)
}

// SearchMemories returns memory chunks semantically relevant to the query,
// restricted to the given user. Used to inject relevant personal context
// into Recall prompts.
func (s *Service) SearchMemories(query string, userID int64, limit int) ([]MemoryHit, error) {
	if !s.IsEnabled() || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	vec, err := llm.EmbedText(s.embedKey, query)
	if err != nil {
		return nil, err
	}
	filter := qdrant.MustMatch("user_id", userID)
	results, err := s.qd.Search(qdrant.CollectionMemories, vec, limit, filter)
	if err != nil {
		return nil, err
	}
	hits := make([]MemoryHit, 0, len(results))
	for _, r := range results {
		h := MemoryHit{Score: r.Score}
		if v, ok := r.Payload["memory_id"].(float64); ok {
			h.MemoryID = int64(v)
		}
		if v, ok := r.Payload["title"].(string); ok {
			h.Title = v
		}
		if v, ok := r.Payload["chunk_text"].(string); ok {
			h.ChunkText = v
		}
		hits = append(hits, h)
	}
	return hits, nil
}

type MemoryHit struct {
	MemoryID  int64   `json:"memory_id"`
	Title     string  `json:"title"`
	ChunkText string  `json:"chunk_text"`
	Score     float64 `json:"score"`
}

// DeleteMemory removes all chunks for a memory.
func (s *Service) DeleteMemory(memoryID int64) error {
	if !s.IsEnabled() {
		return nil
	}
	return s.qd.DeleteByFilter(qdrant.CollectionMemories, qdrant.MustMatch("memory_id", memoryID))
}
