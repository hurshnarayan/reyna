package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeminiEmbedDim is the vector size for Gemini text-embedding-004.
const GeminiEmbedDim = 768

// EmbedModel is the Gemini embedding model used across Reyna.
const EmbedModel = "text-embedding-004"

// EmbedText returns a vector embedding for the given text using Gemini.
// Returns a zero-length slice if apiKey is empty (caller should treat as disabled).
// Truncates text to ~10k runes to stay well under model input limits.
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

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s", EmbedModel, apiKey)
	body := map[string]any{
		"model": "models/" + EmbedModel,
		"content": map[string]any{
			"parts": []map[string]string{{"text": text}},
		},
	}
	jsonBody, _ := json.Marshal(body)

	respBody, status, err := doGeminiRequestWithRetry(url, jsonBody)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("embed API error %d: %s", status, string(respBody))
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
		return nil, fmt.Errorf("empty embedding from gemini")
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
