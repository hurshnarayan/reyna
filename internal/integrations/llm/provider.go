package llm

import (
	"bytes"
	"encoding/base64"
	"errors"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// isRetryableStatus returns true for transient HTTP errors that warrant a retry.
// 429 = rate limit, 500 = internal error, 502/503/504 = upstream / overloaded.
func isRetryableStatus(code int) bool {
	return code == 429 || code == 500 || code == 502 || code == 503 || code == 504
}

// doGeminiRequestWithRetry posts to Gemini with up to 4 attempts on transient
// errors (1s, 2s, 4s backoff). Returns the final response body + status.
func doGeminiRequestWithRetry(url string, jsonBody []byte) ([]byte, int, error) {
	gate.wait()
	var lastBody []byte
	var lastStatus int
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastStatus = 0
			if attempt < 3 {
				wait := time.Duration(1<<attempt) * time.Second
				log.Printf("[LLM] Gemini network error: %v — retrying in %v (attempt %d/4)", err, wait, attempt+2)
				time.Sleep(wait)
				continue
			}
			return nil, 0, err
		}
		lastBody, _ = io.ReadAll(resp.Body)
		lastStatus = resp.StatusCode
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return lastBody, 200, nil
		}
		// 429 is a quota, not a hiccup.
		//
		// The free tier limit is per minute, so a backoff of one, two and four
		// seconds cannot clear it, and Google's own response asks for twenty
		// three. Retrying only multiplies the wait: five documents in one
		// query took ninety eight seconds, almost all of it sleeping between
		// attempts that were never going to succeed. Fail immediately and let
		// the caller decide.
		if resp.StatusCode == 429 {
			log.Printf("[LLM] Gemini quota exhausted, not retrying")
			return lastBody, lastStatus, nil
		}
		if !isRetryableStatus(resp.StatusCode) || attempt == 3 {
			return lastBody, lastStatus, nil
		}
		wait := time.Duration(1<<attempt) * time.Second
		log.Printf("[LLM] Gemini %d — retrying in %v (attempt %d/4)", resp.StatusCode, wait, attempt+2)
		time.Sleep(wait)
	}
	return lastBody, lastStatus, nil
}

// Provider is the unified interface for all LLM backends.
type Provider interface {
	// Complete sends a prompt and returns the text response.
	Complete(prompt string, maxTokens int) (string, error)
	// CompleteWithDoc sends a prompt along with a base64-encoded document for content extraction.
	// Falls back to Complete with filename-only analysis if the provider doesn't support documents.
	CompleteWithDoc(prompt string, fileData []byte, mimeType string, maxTokens int) (string, error)
	// Embed returns a vector embedding for the given text (e.g. 768-dim float32 vector).
	Embed(text string) ([]float32, error)
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
func (n *noop) Embed(text string) ([]float32, error) {
	return nil, nil
}
func (n *noop) Name() string    { return "none" }
func (n *noop) IsEnabled() bool { return false }

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

func (c *claudeProvider) Embed(text string) ([]float32, error) {
	return nil, errors.New("claude embedding not supported")
}

// geminiModels is the order models are tried in.
//
// The free tier meters twenty requests a day per project per model, so one
// model alone reads twenty documents and then refuses for the rest of the day.
// Each name has its own separate allowance, so falling through the list turns
// twenty a day into twenty a day per model, which is the difference between a
// library that becomes searchable and one that does not.
//
// Overridable with GEMINI_MODEL, comma separated, because model names change
// faster than this codebase does. The default learned that the hard way:
// gemini-2.5-flash is retired for newly created projects and answers 404 with
// "no longer available to new users", which reads like a bad key.
func geminiModels() []string {
	if m := os.Getenv("GEMINI_MODEL"); m != "" {
		out := []string{}
		for _, part := range strings.Split(m, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{
		"gemini-3.6-flash",
		"gemini-3.1-flash-lite",
		"gemini-3-flash-preview",
	}
}

// geminiPost tries each model in turn, moving on when one is out of quota.
//
// Only quota moves it along. Any other failure is returned as is, because
// retrying a malformed request against three models turns one clear error into
// three confusing ones.
// ErrOutOfQuota means every configured model has spent its daily allowance.
//
// Returned instead of attempting a call, so a caller can fall back at once
// rather than discovering it request by request.
var ErrOutOfQuota = errors.New("all Gemini models are out of quota for today")

// quotaWall remembers which models are spent, and until when.
//
// Without it an exhausted allowance is rediscovered on every single call. Each
// discovery costs a slot in the rate gate, which permits fifteen a minute, and
// one question makes several calls across several models. So once the day's
// quota was gone a question spent over two minutes queueing for permission to
// receive 429s it already knew were coming, and only then showed the fallback
// reply. The wall is per model because the limit is per model, and it is what
// makes running out of allowance feel like an answer rather than a hang.
var quotaWall = struct {
	mu    sync.Mutex
	until map[string]time.Time
}{until: map[string]time.Time{}}

func modelBlocked(model string) bool {
	quotaWall.mu.Lock()
	defer quotaWall.mu.Unlock()
	t, ok := quotaWall.until[model]
	if !ok {
		return false
	}
	if time.Now().After(t) {
		delete(quotaWall.until, model)
		return false
	}
	return true
}

// blockModel records that this model is spent, reading the response to tell a
// daily allowance from a per-minute burst.
//
// A per-minute limit clears on its own in well under a minute and must not
// retire a model for the rest of the day. A daily one will not clear until
// Google's reset, which is midnight Pacific rather than local midnight; that
// distinction is why "it will reset tomorrow" was true and "it should have
// reset by now" was not.
func blockModel(model string, body []byte) {
	until := time.Now().Add(70 * time.Second)
	daily := bytes.Contains(body, []byte("PerDay"))
	if daily {
		until = nextPacificMidnight()
	}
	quotaWall.mu.Lock()
	quotaWall.until[model] = until
	quotaWall.mu.Unlock()
	if daily {
		log.Printf("[LLM] %s has spent its daily allowance, skipping until %s",
			model, until.Format(time.RFC1123))
	} else {
		log.Printf("[LLM] %s is rate limited, skipping for a minute", model)
	}
}

// QuotaExhaustedUntil reports whether every configured model has spent its
// daily allowance, and when the first of them comes back.
//
// Exported so a request can find this out before doing any work. Discovering
// it at the point of the model call means the search, the Drive walk and the
// document reads have all already happened, and every one of them is wasted:
// there is nothing left that can turn them into an answer. Asked up front, the
// same fact becomes an immediate, honest reply.
//
// Returns false when no model has been walled, which includes the ordinary
// case of never having hit a limit at all.
func QuotaExhaustedUntil() (time.Time, bool) {
	models := geminiModels()
	if len(models) == 0 {
		return time.Time{}, false
	}
	quotaWall.mu.Lock()
	defer quotaWall.mu.Unlock()

	now := time.Now()
	var soonest time.Time
	for _, m := range models {
		t, ok := quotaWall.until[m]
		if !ok || now.After(t) {
			return time.Time{}, false
		}
		if soonest.IsZero() || t.Before(soonest) {
			soonest = t
		}
	}
	return soonest, true
}

// nextPacificMidnight is when Google's free tier daily counters roll over.
func nextPacificMidnight() time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Without the zone database, wait an hour and find out by asking.
		return time.Now().Add(time.Hour)
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
}

func geminiPost(apiKey string, jsonBody []byte) ([]byte, int, error) {
	var lastBody []byte
	var lastStatus int
	var lastErr error
	tried := 0
	for _, model := range geminiModels() {
		// Skipped before the rate gate, not after. Waiting for permission to
		// make a call that is certain to fail is the whole cost being avoided.
		if modelBlocked(model) {
			continue
		}
		tried++
		url := fmt.Sprintf(
			"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
			model, apiKey,
		)
		body, status, err := doGeminiRequestWithRetry(url, jsonBody)
		if err == nil && status == 200 {
			return body, status, nil
		}
		lastBody, lastStatus, lastErr = body, status, err
		if status != 429 {
			return body, status, err
		}
		blockModel(model, body)
	}
	if tried == 0 {
		return nil, 429, ErrOutOfQuota
	}
	return lastBody, lastStatus, lastErr
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

	// Only force application/json output when the prompt explicitly asks for
	// JSON. Free-form answers (Q&A, NLP reply generation) must stay plain text
	// — otherwise Gemini wraps the response in a {"answer": "..."} envelope.
	wantJSON := strings.Contains(prompt, "JSON") || strings.Contains(prompt, "json")
	// No thinkingConfig.
	//
	// thinkingBudget: 0 turned off deliberation on 2.5 and saved a little
	// latency. Gemini 3 rejects the field outright with 400 INVALID_ARGUMENT,
	// and because it was sent on every call, every document read failed while
	// looking like a bad key or a bad file. Not worth reintroducing per model:
	// the saving was small and the failure mode was silent.
	genConfig := map[string]interface{}{
		"maxOutputTokens": maxTokens,
		"temperature":     0.3,
	}
	if wantJSON {
		genConfig["responseMimeType"] = "application/json"
		genConfig["temperature"] = 0.2
	}

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": genConfig,
	}
	jsonBody, _ := json.Marshal(body)

	respBody, status, err := geminiPost(g.apiKey, jsonBody)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("gemini API error %d: %s", status, string(respBody))
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
		"generationConfig": func() map[string]interface{} {
			cfg := map[string]interface{}{
				"maxOutputTokens": maxTokens,
				"temperature":     0.3,
				// Reasoning tokens come out of the same budget as the answer.
				//
				// On the Gemini 3 models this call runs against, a document
				// transcription would spend the entire output allowance
				// thinking and return a candidate with no text in it at all.
				// The caller saw "unexpected end of JSON input" and treated the
				// document as unreadable, which is how a perfectly ordinary PDF
				// came to be retired. Transcribing a page is not a task that
				// needs deliberation, so none is bought.
				"thinkingConfig": map[string]interface{}{"thinkingBudget": 0},
			}
			if strings.Contains(prompt, "JSON") || strings.Contains(prompt, "json") {
				cfg["responseMimeType"] = "application/json"
				cfg["temperature"] = 0.2
			}
			return cfg
		}(),
	}
	jsonBody, _ := json.Marshal(body)

	respBody, status, err := geminiPost(g.apiKey, jsonBody)
	if err != nil {
		return "", err
	}
	if status != 200 {
		log.Printf("[LLM] Gemini doc API error %d, falling back to text-only", status)
		return g.Complete(prompt, maxTokens)
	}

	var r struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", err
	}
	if len(r.Candidates) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}
	// Say why it was empty rather than returning "" for the caller to
	// misdiagnose. An answer cut off at the token ceiling, a safety block and
	// a document with no text in it are three different problems, and the one
	// thing they must not all look like is the last of those.
	c := r.Candidates[0]
	var text string
	for _, part := range c.Content.Parts {
		text += part.Text
	}
	if strings.TrimSpace(text) == "" {
		reason := c.FinishReason
		if reason == "" {
			reason = "no reason given"
		}
		return "", fmt.Errorf("gemini returned no text (finishReason %s)", reason)
	}
	if c.FinishReason == "MAX_TOKENS" {
		log.Printf("[LLM] Gemini doc reply hit the token ceiling; keeping the %d chars it returned", len(text))
	}
	return text, nil
}

func (g *geminiProvider) Embed(text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if len(text) > 8000 {
		text = text[:8000]
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent?key=%s", g.apiKey)
	body := map[string]interface{}{
		"model": "models/gemini-embedding-001",
		"content": map[string]interface{}{
			"parts": []map[string]string{
				{"text": text},
			},
		},
		"outputDimensionality": 768,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	respBody, status, err := doGeminiRequestWithRetry(url, jsonBody)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("gemini embed error %d: %s", status, string(respBody))
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
		return nil, fmt.Errorf("empty embedding returned from Gemini")
	}
	return r.Embedding.Values, nil
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

func (x *grokProvider) Embed(text string) ([]float32, error) {
	return nil, errors.New("grok embedding not supported")
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

func (o *openaiProvider) Embed(text string) ([]float32, error) {
	return nil, errors.New("openai embedding not supported")
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

// ── Rate limiting ──

// gate paces every Gemini call in the process.
//
// The free tier allows twenty requests a minute. Uploading a phone's backlog
// fires one call per file as fast as the files arrive, which exhausts the
// minute in seconds, fails the rest, and leaves documents stored but never
// read. A hundred and thirty one files were ingested that way and exactly one
// ended up with any extracted text.
//
// Waiting is strictly better than failing here. A file read a minute late is
// still searchable forever; a file that returned 429 is never read again,
// because nothing retries it. The limit is set below the real one so that an
// interactive question is not starved by a backlog running behind it.
var gate = newRateGate(15, time.Minute)

type rateGate struct {
	mu     sync.Mutex
	times  []time.Time
	limit  int
	window time.Duration
}

func newRateGate(limit int, window time.Duration) *rateGate {
	return &rateGate{limit: limit, window: window}
}

// wait blocks until another call is allowed.
func (g *rateGate) wait() {
	for {
		g.mu.Lock()
		cutoff := time.Now().Add(-g.window)
		kept := g.times[:0]
		for _, t := range g.times {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		g.times = kept
		if len(g.times) < g.limit {
			g.times = append(g.times, time.Now())
			g.mu.Unlock()
			return
		}
		sleep := time.Until(g.times[0].Add(g.window)) + 50*time.Millisecond
		g.mu.Unlock()
		if sleep <= 0 {
			sleep = 100 * time.Millisecond
		}
		time.Sleep(sleep)
	}
}
