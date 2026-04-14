// Package qdrant is a minimal REST client for Qdrant vector search.
// Only the subset Reyna needs: collection creation, upsert, search, delete.
// No external dependencies — uses stdlib net/http so go.mod stays clean.
package qdrant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New returns a Qdrant client. Pass an empty url to disable (returns nil).
func New(url, apiKey string) *Client {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	return &Client{
		baseURL: strings.TrimRight(url, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// IsEnabled returns true if the client is configured.
func (c *Client) IsEnabled() bool { return c != nil && c.baseURL != "" }

func (c *Client) do(method, path string, body any) ([]byte, int, error) {
	if c == nil {
		return nil, 0, fmt.Errorf("qdrant client is nil")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// EnsureCollection creates the collection if it doesn't exist. Idempotent.
// vectorSize must match the embedding model (768 for Gemini text-embedding-004).
func (c *Client) EnsureCollection(name string, vectorSize int) error {
	// Check existence
	_, status, err := c.do("GET", "/collections/"+name, nil)
	if err != nil {
		return err
	}
	if status == 200 {
		return nil
	}
	// Create
	body := map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	}
	respBody, status, err := c.do("PUT", "/collections/"+name, body)
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("create collection %s: status %d: %s", name, status, string(respBody))
	}
	log.Printf("[Qdrant] Created collection %s (vector size %d)", name, vectorSize)
	return nil
}

// Point is a single vector with ID and payload for upsert.
type Point struct {
	ID      any            `json:"id"` // integer or UUID string
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload,omitempty"`
}

// Upsert inserts or updates points in a collection.
func (c *Client) Upsert(collection string, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	body := map[string]any{"points": points}
	respBody, status, err := c.do("PUT", "/collections/"+collection+"/points?wait=true", body)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("upsert: status %d: %s", status, string(respBody))
	}
	return nil
}

// ScoredPoint is one search result.
type ScoredPoint struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

// Filter helpers — Qdrant uses a JSON filter language. We wrap the common cases.
// MustMatch builds a single "must match key == value" filter.
func MustMatch(key string, value any) map[string]any {
	return map[string]any{
		"must": []map[string]any{
			{"key": key, "match": map[string]any{"value": value}},
		},
	}
}

// MustMatchAny builds a filter requiring key's value to match any of values.
func MustMatchAny(key string, values []any) map[string]any {
	return map[string]any{
		"must": []map[string]any{
			{"key": key, "match": map[string]any{"any": values}},
		},
	}
}

// Search performs vector similarity search with optional filter.
func (c *Client) Search(collection string, vector []float32, limit int, filter map[string]any) ([]ScoredPoint, error) {
	if limit <= 0 {
		limit = 10
	}
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if filter != nil {
		body["filter"] = filter
	}
	respBody, status, err := c.do("POST", "/collections/"+collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("search: status %d: %s", status, string(respBody))
	}
	var r struct {
		Result []ScoredPoint `json:"result"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return nil, err
	}
	return r.Result, nil
}

// Delete removes points by ID.
func (c *Client) Delete(collection string, ids []any) error {
	if len(ids) == 0 {
		return nil
	}
	body := map[string]any{"points": ids}
	respBody, status, err := c.do("POST", "/collections/"+collection+"/points/delete?wait=true", body)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("delete: status %d: %s", status, string(respBody))
	}
	return nil
}

// DeleteByFilter removes all points matching a filter.
func (c *Client) DeleteByFilter(collection string, filter map[string]any) error {
	body := map[string]any{"filter": filter}
	respBody, status, err := c.do("POST", "/collections/"+collection+"/points/delete?wait=true", body)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("delete-by-filter: status %d: %s", status, string(respBody))
	}
	return nil
}
