package llm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// Provider is the unified interface for all LLM backends.
type Provider interface {
	// Complete sends a prompt and returns the text response.
	Complete(prompt string, maxTokens int) (string, error)
	// CompleteWithDoc sends a prompt along with a base64-encoded document for content extraction.
	// Falls back to Complete with filename-only analysis if the provider doesn't support documents.
	CompleteWithDoc(prompt string, fileData []byte, mimeType string, maxTokens int) (string, error)
	// Name returns the provider name for logging.
	Name() string
	// IsEnabled returns true if the provider has a valid API key.
	IsEnabled() bool
}

// New creates the appropriate provider based on config.
// providerName: "claude" | "gemini" | "grok"
// Falls back to a no-op provider if no key is set.
func New(providerName, apiKey string) Provider {
	if apiKey == "" {
		return &noop{}
	}

	switch strings.ToLower(providerName) {
	case "gemini", "google":
		return &geminiProvider{apiKey: apiKey}
	case "grok", "xai":
		return &grokProvider{apiKey: apiKey}
	case "openai", "gpt":
		return &openaiProvider{apiKey: apiKey}
	case "claude", "anthropic", "":
		return &claudeProvider{apiKey: apiKey}
	default:
		log.Printf("[LLM] Unknown provider %q, falling back to Claude", providerName)
		return &claudeProvider{apiKey: apiKey}
	}
}

// ── No-op provider (keyword-only mode) ──

type noop struct{}

func (n *noop) Complete(prompt string, maxTokens int) (string, error) {
	return "", fmt.Errorf("no LLM provider configured")
}
func (n *noop) CompleteWithDoc(prompt string, fileData []byte, mimeType string, maxTokens int) (string, error) {
	return "", fmt.Errorf("no LLM provider configured")
}
func (n *noop) Name() string      { return "none" }
func (n *noop) IsEnabled() bool   { return false }

// ── Claude (Anthropic) ──

type claudeProvider struct {
	apiKey string
}

func (c *claudeProvider) Name() string    { return "claude" }
func (c *claudeProvider) IsEnabled() bool { return c.apiKey != "" }

func (c *claudeProvider) Complete(prompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 300
	}
	body := map[string]interface{}{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("claude API error %d: %s", resp.StatusCode, string(respBody))
	}

	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", err
	}
	if len(r.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}
	return r.Content[0].Text, nil
}

func (c *claudeProvider) CompleteWithDoc(prompt string, fileData []byte, mimeType string, maxTokens int) (string, error) {
	if len(fileData) == 0 {
		return c.Complete(prompt, maxTokens)
	}
	if maxTokens <= 0 {
		maxTokens = 500
	}

	// Map mime type to Claude's supported document types
	mediaType := mimeType
	docType := "document"
	if strings.Contains(mimeType, "pdf") {
		mediaType = "application/pdf"
	} else if strings.Contains(mimeType, "image") {
		docType = "image"
	} else {
		// For unsupported types (docx etc), fall back to text prompt
		return c.Complete(prompt, maxTokens)
	}

	b64 := base64.StdEncoding.EncodeToString(fileData)

	// Build multimodal message with document block
	body := map[string]interface{}{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": maxTokens,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": docType,
						"source": map[string]interface{}{
							"type":       "base64",
							"media_type": mediaType,
							"data":       b64,
						},
					},
					{
						"type": "text",
						"text": prompt,
					},
				},
			},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		log.Printf("[LLM] Claude doc API error %d, falling back to text-only", resp.StatusCode)
		return c.Complete(prompt, maxTokens)
	}

	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", err
	}
	if len(r.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}
	return r.Content[0].Text, nil
}

// ── Gemini (Google AI Studio) ──

type geminiProvider struct {
	apiKey string
}

func (g *geminiProvider) Name() string    { return "gemini" }
func (g *geminiProvider) IsEnabled() bool { return g.apiKey != "" }

func (g *geminiProvider) Complete(prompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 300
	}

	// Gemini API: POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", g.apiKey)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens":  maxTokens,
			"temperature":      0.2,
			"responseMimeType": "application/json",
			"thinkingConfig":   map[string]interface{}{"thinkingBudget": 0},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(respBody))
	}

	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", err
	}
	if len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}
	return r.Candidates[0].Content.Parts[0].Text, nil
}

func (g *geminiProvider) CompleteWithDoc(prompt string, fileData []byte, mimeType string, maxTokens int) (string, error) {
	if len(fileData) == 0 {
		return g.Complete(prompt, maxTokens)
	}
	if maxTokens <= 0 {
		maxTokens = 500
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", g.apiKey)

	b64 := base64.StdEncoding.EncodeToString(fileData)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"inlineData": map[string]string{
							"mimeType": mimeType,
							"data":     b64,
						},
					},
					{
						"text": prompt,
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens":  maxTokens,
			"temperature":      0.2,
			"responseMimeType": "application/json",
			"thinkingConfig":   map[string]interface{}{"thinkingBudget": 0},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		log.Printf("[LLM] Gemini doc API error %d, falling back to text-only", resp.StatusCode)
		return g.Complete(prompt, maxTokens)
	}

	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", err
	}
	if len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}
	return r.Candidates[0].Content.Parts[0].Text, nil
}

// ── Grok (xAI) — OpenAI-compatible API ──

type grokProvider struct {
	apiKey string
}

func (x *grokProvider) Name() string    { return "grok" }
func (x *grokProvider) IsEnabled() bool { return x.apiKey != "" }

func (x *grokProvider) Complete(prompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 300
	}

	// xAI uses OpenAI-compatible chat completions endpoint
	body := map[string]interface{}{
		"model":      "grok-3-mini-fast",
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", "https://api.x.ai/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+x.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("grok API error %d: %s", resp.StatusCode, string(respBody))
	}

	// OpenAI-compatible response format
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", err
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("empty response from Grok")
	}
	return r.Choices[0].Message.Content, nil
}

// Grok doesn't support document blocks — fallback to text-only
func (x *grokProvider) CompleteWithDoc(prompt string, fileData []byte, mimeType string, maxTokens int) (string, error) {
	return x.Complete(prompt, maxTokens)
}

// ── OpenAI (GPT) ──

type openaiProvider struct {
	apiKey string
}

func (o *openaiProvider) Name() string    { return "openai" }
func (o *openaiProvider) IsEnabled() bool { return o.apiKey != "" }

func (o *openaiProvider) Complete(prompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 300
	}

	body := map[string]interface{}{
		"model":      "gpt-4o-mini",
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(respBody))
	}

	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", err
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("empty response from OpenAI")
	}
	return r.Choices[0].Message.Content, nil
}

// OpenAI doesn't support document blocks in the same way — fallback to text-only
func (o *openaiProvider) CompleteWithDoc(prompt string, fileData []byte, mimeType string, maxTokens int) (string, error) {
	return o.Complete(prompt, maxTokens)
}

// ── Helpers ──

// CleanJSON strips markdown code fences from LLM responses that wrap JSON.
func CleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
