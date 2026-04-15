package llm

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
)

// GeminiEmbedDim is the vector size we ask Gemini to return. Pinned to
// 768 because our Qdrant collections were created with that size —
// changing it requires recreating the collections.
const GeminiEmbedDim = 768

// EmbedModel is the canonical name we report in logs. The real model
// used per call is whatever the embedCandidates list discovers works
// against the configured API key. Google has renamed embedding models
// + moved them between v1 and v1beta several times; we probe rather
// than hard-code a single name.
const EmbedModel = "auto"

// embedCandidate is one (apiVersion, modelName, supportsOutputDim) combo
// we'll try in order. The first one that returns 200 for any call gets
// remembered in `embedActive` and used for everything after.
type embedCandidate struct {
	apiVersion string
	model      string
	// Some models (text-embedding-004) ignore outputDimensionality and
	// always return their default size. Others (gemini-embedding-001)
	// require it to downsample. We try with-dim first; on success, we
	// verify the returned vector is the size we expected.
	withDim bool
}

// Per https://ai.google.dev/gemini-api/docs/embeddings — embeddings live
// only on v1beta. Order: latest preview → GA → legacy.
var embedCandidates = []embedCandidate{
	{"v1beta", "gemini-embedding-2-preview", true}, // newest, multimodal preview
	{"v1beta", "gemini-embedding-001", true},       // GA, recommended
	{"v1beta", "text-embedding-004", false},        // legacy fallback
	{"v1beta", "embedding-001", false},             // older legacy
}

var (
	embedActiveMu sync.Mutex
	embedActive   *embedCandidate // first candidate that worked, sticky
)

// EmbedText returns a vector embedding for the given text using Gemini.
// Tries multiple model+endpoint combinations on first call until one
// returns a 200, then sticks with whichever worked. Subsequent calls
// fast-path through the remembered candidate.
func EmbedText(apiKey, text string) ([]float32, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini api key not set")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}
	if n := len([]rune(text)); n > 10000 {
		r := []rune(text)
		text = string(r[:10000])
	}

	embedActiveMu.Lock()
	pinned := embedActive
	embedActiveMu.Unlock()

	tryList := embedCandidates
	if pinned != nil {
		// Try the known-good one first; fall through to the others on
		// failure (covers the case where Google retires the active model).
		tryList = append([]embedCandidate{*pinned}, embedCandidates...)
	}

	var lastErr error
	for _, c := range tryList {
		vec, err := callEmbed(apiKey, c, text)
		if err == nil {
			if pinned == nil || pinned.model != c.model || pinned.apiVersion != c.apiVersion {
				embedActiveMu.Lock()
				embedActive = &c
				embedActiveMu.Unlock()
				log.Printf("[EMBED] using %s/%s", c.apiVersion, c.model)
			}
			return vec, nil
		}
		lastErr = err
		// Retry the next candidate only on 404 (model-not-found). For
		// 429/5xx the inner doGeminiRequestWithRetry already retried;
		// switching models won't help.
		if !strings.Contains(err.Error(), "404") {
			return nil, err
		}
	}
	return nil, fmt.Errorf("no embedding model worked for this api key: %w", lastErr)
}

func callEmbed(apiKey string, c embedCandidate, text string) ([]float32, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/%s/models/%s:embedContent?key=%s", c.apiVersion, c.model, apiKey)
	body := map[string]any{
		"model": "models/" + c.model,
		"content": map[string]any{
			"parts": []map[string]string{{"text": text}},
		},
	}
	if c.withDim {
		body["outputDimensionality"] = GeminiEmbedDim
	}
	jsonBody, _ := json.Marshal(body)
	respBody, status, err := doGeminiRequestWithRetry(url, jsonBody)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("embed API error %d (%s/%s): %s", status, c.apiVersion, c.model, string(respBody))
	}
	var r struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return nil, err
	}
	if len(r.Embedding.Values) == 0 {
		return nil, fmt.Errorf("empty embedding from gemini (%s/%s)", c.apiVersion, c.model)
	}
	// Dimension mismatch with our Qdrant collection would silently corrupt
	// search later; surface it now.
	if len(r.Embedding.Values) != GeminiEmbedDim {
		return nil, fmt.Errorf("embed dim mismatch: got %d, want %d (%s/%s)",
			len(r.Embedding.Values), GeminiEmbedDim, c.apiVersion, c.model)
	}
	return r.Embedding.Values, nil
}

// ChunkText splits text into chunks of approximately chunkSize runes, trying
// to break on paragraph/sentence boundaries. Used for embedding large memories
// (e.g., a full syllabus) where a single vector won't capture everything well.
func ChunkText(text string, chunkSize int) []string {
	if chunkSize <= 0 {
		chunkSize = 1500
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	start := 0
	for start < len(runes) {
		end := start + chunkSize
		if end >= len(runes) {
			chunks = append(chunks, strings.TrimSpace(string(runes[start:])))
			break
		}
		// Prefer to break on a paragraph or sentence boundary within the last
		// 200 chars of the window so chunks stay coherent.
		boundary := end
		for i := end; i > end-200 && i > start; i-- {
			if runes[i] == '\n' || runes[i] == '.' {
				boundary = i + 1
				break
			}
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[start:boundary])))
		start = boundary
	}
	return chunks
}
