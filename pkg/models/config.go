package models

// Config represents the application configuration
type Config struct {
	Proxy     ProxyConfig     `mapstructure:"proxy"`
	API       APIConfig       `mapstructure:"api"`
	TLS       TLSConfig       `mapstructure:"tls"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Inference InferenceConfig `mapstructure:"inference"`
	LLM       LLMConfig       `mapstructure:"llm"`
	MCP       MCPConfig       `mapstructure:"mcp"`
	Logging   LoggingConfig   `mapstructure:"logging"`
}

// ProxyConfig contains proxy server settings
type ProxyConfig struct {
	Listen        string   `mapstructure:"listen"`
	UpstreamProxy string   `mapstructure:"upstream_proxy"`
	Timeout       int      `mapstructure:"timeout"` // seconds
	SkipHosts     []string `mapstructure:"skip_hosts"`
}

// APIConfig contains REST API settings
type APIConfig struct {
	Listen      string   `mapstructure:"listen"`
	CORSOrigins []string `mapstructure:"cors_origins"`
}

// TLSConfig contains TLS/CA settings
type TLSConfig struct {
	CACertPath   string `mapstructure:"ca_cert"`
	CAKeyPath    string `mapstructure:"ca_key"`
	CertCacheDir string `mapstructure:"cert_cache_dir"`
}

// StorageConfig contains database settings
type StorageConfig struct {
	Path        string `mapstructure:"path"`
	MaxCaptures int    `mapstructure:"max_captures"`
}

// InferenceConfig contains schema inference settings
type InferenceConfig struct {
	Enabled         bool `mapstructure:"enabled"`
	MinSamples      int  `mapstructure:"min_samples"`
	MaxEnumValues   int  `mapstructure:"max_enum_values"`
	EnumThreshold   int  `mapstructure:"enum_threshold"` // percentage
	InferOnCapture  bool `mapstructure:"infer_on_capture"`
}

// LLMConfig contains LLM provider settings
type LLMConfig struct {
	Provider    string  `mapstructure:"provider"`    // "ollama", "openai", "anthropic", "openai-compatible"
	APIKey      string  `mapstructure:"api_key"`     // API key (not needed for Ollama)
	BaseURL     string  `mapstructure:"base_url"`    // Custom endpoint URL
	Model       string  `mapstructure:"model"`       // Model name
	MaxTokens   int     `mapstructure:"max_tokens"`  // Max tokens for response
	Temperature float64 `mapstructure:"temperature"` // Temperature (0-1)
}

// MCPConfig contains MCP server settings
type MCPConfig struct {
	DBPath string `mapstructure:"db_path"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Proxy: ProxyConfig{
			Listen:  ":8080",
			Timeout: 30,
		},
		API: APIConfig{
			Listen:      ":9090",
			CORSOrigins: []string{"http://localhost:3000"},
		},
		TLS: TLSConfig{
			CACertPath:   "./configs/ca/ca.crt",
			CAKeyPath:    "./configs/ca/ca.key",
			CertCacheDir: "./configs/ca/cache",
		},
		Storage: StorageConfig{
			Path:        "./data/proxy.db",
			MaxCaptures: 100000,
		},
		Inference: InferenceConfig{
			Enabled:        true,
			MinSamples:     3,
			MaxEnumValues:  10,
			EnumThreshold:  50,
			InferOnCapture: false,
		},
		LLM: LLMConfig{
			Provider:    "ollama",
			BaseURL:     "http://localhost:11434/v1",
			Model:       "llama3.1:8b",
			MaxTokens:   4096,
			Temperature: 0.1,
		},
		MCP: MCPConfig{
			DBPath: "./data/proxy.db",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
