package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port              string
	DatabaseURL       string
	JWTSecret         string
	GoogleClientID    string
	GoogleSecret      string
	GoogleRedirectURL string
	FrontendURL       string
	WhatsAppMode      string // "baileys" or "mock"
	AutoCommitHours   int    // Hours before staged files auto-commit (default 24)

	// LLM provider settings — swappable between Claude, Gemini, Grok, OpenAI
	// LLM_PROVIDER: "claude" | "gemini" | "grok" | "openai" (default: auto-detect from available keys)
	// Set the corresponding API key for your chosen provider.
	LLMProvider     string
	AnthropicAPIKey string // ANTHROPIC_API_KEY — for Claude Haiku
	GeminiAPIKey    string // GEMINI_API_KEY   — for Gemini Flash (free tier: 1500 req/day)
	XAIAPIKey       string // XAI_API_KEY      — for Grok
	OpenAIAPIKey    string // OPENAI_API_KEY   — for GPT-4o-mini

	// Qdrant — vector DB for Reyna's Recall (semantic search) and Memory.
	// QDRANT_URL empty = semantic search disabled, falls back to keyword-only.
	QdrantURL    string // e.g. https://xxx.cloud.qdrant.io:6333 or http://localhost:6333
	QdrantAPIKey string // required for Qdrant Cloud, empty for local

	// Vapi — voice agent webhooks. VAPI_WEBHOOK_SECRET is the shared token we
	// verify on every incoming tool call. VAPI_PUBLIC_KEY is exposed to the
	// frontend to initialize the web SDK.
	VapiWebhookSecret string // VAPI_WEBHOOK_SECRET
	VapiPublicKey     string // VAPI_PUBLIC_KEY (web SDK, safe to expose)
	VapiAssistantID   string // VAPI_ASSISTANT_ID (assistant to dial on web calls)
}

func Load() *Config {
	ach, _ := strconv.Atoi(getEnv("AUTO_COMMIT_HOURS", "24"))
	if ach <= 0 {
		ach = 24
	}

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       getEnv("DATABASE_URL", "./reyna.db"),
		JWTSecret:         getEnv("JWT_SECRET", "reyna-dev-secret-change-in-prod"),
		GoogleClientID:    getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleSecret:      getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL: getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/auth/google/callback"),
		FrontendURL:       getEnv("FRONTEND_URL", "http://localhost:5173"),
		WhatsAppMode:      getEnv("WHATSAPP_MODE", "mock"),
		AutoCommitHours:   ach,

		LLMProvider:     getEnv("LLM_PROVIDER", ""),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		GeminiAPIKey:    getEnv("GEMINI_API_KEY", ""),
		XAIAPIKey:       getEnv("XAI_API_KEY", ""),
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),

		QdrantURL:    getEnv("QDRANT_URL", ""),
		QdrantAPIKey: getEnv("QDRANT_API_KEY", ""),

		VapiWebhookSecret: getEnv("VAPI_WEBHOOK_SECRET", ""),
		VapiPublicKey:     getEnv("VAPI_PUBLIC_KEY", ""),
		VapiAssistantID:   getEnv("VAPI_ASSISTANT_ID", ""),
	}

	// Auto-detect provider from available keys if not explicitly set
	if cfg.LLMProvider == "" {
		switch {
		case cfg.AnthropicAPIKey != "":
			cfg.LLMProvider = "claude"
		case cfg.GeminiAPIKey != "":
			cfg.LLMProvider = "gemini"
		case cfg.OpenAIAPIKey != "":
			cfg.LLMProvider = "openai"
		case cfg.XAIAPIKey != "":
			cfg.LLMProvider = "grok"
		}
	}

	return cfg
}

// LLMAPIKey returns the API key for the active LLM provider.
func (c *Config) LLMAPIKey() string {
	switch c.LLMProvider {
	case "gemini", "google":
		return c.GeminiAPIKey
	case "grok", "xai":
		return c.XAIAPIKey
	case "openai", "gpt":
		return c.OpenAIAPIKey
	default:
		return c.AnthropicAPIKey
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
