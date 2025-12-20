package llm

import (
	"context"
	"fmt"
)

// Provider defines the interface for LLM providers
type Provider interface {
	// Complete sends a prompt and returns the completion
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// Name returns the provider name
	Name() string
}

// CompletionRequest contains the request parameters
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

// CompletionResponse contains the response
type CompletionResponse struct {
	Content      string
	FinishReason string
	TokensUsed   int
}

// Config holds LLM provider configuration
type Config struct {
	Provider    string  `yaml:"provider"`    // "openai", "anthropic", "ollama"
	APIKey      string  `yaml:"api_key"`     // API key (not needed for Ollama)
	BaseURL     string  `yaml:"base_url"`    // Custom endpoint (for Ollama, vLLM, etc.)
	Model       string  `yaml:"model"`       // Model name
	MaxTokens   int     `yaml:"max_tokens"`  // Default max tokens
	Temperature float64 `yaml:"temperature"` // Default temperature
}

// DefaultConfig returns sensible defaults for Ollama
func DefaultConfig() *Config {
	return &Config{
		Provider:    "ollama",
		BaseURL:     "http://localhost:11434/v1",
		Model:       "llama3.1:8b",
		MaxTokens:   4096,
		Temperature: 0.1, // Low temperature for consistent schema inference
	}
}

// NewProvider creates a provider based on config
func NewProvider(cfg *Config) (Provider, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg.APIKey, cfg.Model, "https://api.openai.com/v1", cfg.MaxTokens, cfg.Temperature)
	case "ollama":
		// Ollama uses OpenAI-compatible API
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		return NewOpenAIProvider("", cfg.Model, baseURL, cfg.MaxTokens, cfg.Temperature)
	case "openai-compatible":
		return NewOpenAIProvider(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.MaxTokens, cfg.Temperature)
	case "anthropic":
		return NewAnthropicProvider(cfg.APIKey, cfg.Model, cfg.MaxTokens, cfg.Temperature)
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
}
