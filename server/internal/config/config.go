package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port string
	Env  string
	CORS []string

	JWTSecret      string
	JWTExpiryHours int

	DatabaseURL string
	RedisURL    string

	OpenAIKey          string
	OpenAIDefaultModel string

	AnthropicKey          string
	AnthropicDefaultModel string

	GoogleKey          string
	GoogleDefaultModel string

	// Groq is an OpenAI-compatible provider with a free tier (no card required).
	// It only registers when GroqKey is set.
	GroqKey          string
	GroqDefaultModel string
	GroqBaseURL      string

	// LLMFallbackModels is an optional comma-separated list of
	// "provider:model" candidates tried in order when the agent's configured
	// model fails (e.g. quota exhausted). Empty = automatic chain.
	LLMFallbackModels []string

	CashbackAdapterMode string
	CashbackDBURL       string
	CashbackAPIURL      string
	CashbackAPIKey      string
	// CashbackCacheTTL seconds to cache adapter tool results in memory.
	CashbackCacheTTL int
}

func Load() *Config {
	return &Config{
		Port: getEnv("PORT", "8080"),
		Env:  getEnv("ENV", "development"),
		CORS: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),

		JWTSecret:      getEnv("JWT_SECRET", "dev_secret_change_me"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 72),

		DatabaseURL: getEnv("DATABASE_URL", "postgres://nexus:nexus_secret@localhost:5432/nexus?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),

		OpenAIKey:          getEnv("OPENAI_API_KEY", ""),
		OpenAIDefaultModel: getEnv("OPENAI_DEFAULT_MODEL", "gpt-4.1"),

		AnthropicKey:          getEnv("ANTHROPIC_API_KEY", ""),
		AnthropicDefaultModel: getEnv("ANTHROPIC_DEFAULT_MODEL", "claude-sonnet-4-5"),

		GoogleKey:          getEnv("GOOGLE_API_KEY", ""),
		GoogleDefaultModel: getEnv("GOOGLE_DEFAULT_MODEL", "gemini-2.5-pro"),

		GroqKey:          getEnv("GROQ_API_KEY", ""),
		GroqDefaultModel: getEnv("GROQ_DEFAULT_MODEL", "llama-3.3-70b-versatile"),
		GroqBaseURL:      getEnv("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),

		LLMFallbackModels: splitEnv("LLM_FALLBACK_MODELS", ""),

		CashbackAdapterMode: getEnv("CASHBACK_ADAPTER_MODE", "db"),
		CashbackDBURL:       getEnv("CASHBACK_DB_URL", ""),
		CashbackAPIURL:      getEnv("CASHBACK_API_URL", ""),
		CashbackAPIKey:      getEnv("CASHBACK_API_KEY", ""),
		CashbackCacheTTL:    getEnvInt("CASHBACK_CACHE_TTL", 60),
	}
}

func splitEnv(key, fallback string) []string {
	v := os.Getenv(key)
	if v == "" {
		if fallback == "" {
			return nil
		}
		v = fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
