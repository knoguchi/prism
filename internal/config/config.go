package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"prism/pkg/models"
)

// Load reads configuration from file and environment
func Load(configPath string) (*models.Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("proxy")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	// Read environment variables
	v.SetEnvPrefix("AIPROXY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		// Config file not found, use defaults
	}

	var cfg models.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Check for provider-specific API keys as fallback
	if cfg.LLM.APIKey == "" {
		switch cfg.LLM.Provider {
		case "anthropic":
			cfg.LLM.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		case "openai":
			cfg.LLM.APIKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	defaults := models.DefaultConfig()

	// Proxy defaults
	v.SetDefault("proxy.listen", defaults.Proxy.Listen)
	v.SetDefault("proxy.timeout", defaults.Proxy.Timeout)

	// API defaults
	v.SetDefault("api.listen", defaults.API.Listen)
	v.SetDefault("api.cors_origins", defaults.API.CORSOrigins)

	// TLS defaults
	v.SetDefault("tls.ca_cert", defaults.TLS.CACertPath)
	v.SetDefault("tls.ca_key", defaults.TLS.CAKeyPath)
	v.SetDefault("tls.cert_cache_dir", defaults.TLS.CertCacheDir)

	// Storage defaults
	v.SetDefault("storage.path", defaults.Storage.Path)
	v.SetDefault("storage.max_captures", defaults.Storage.MaxCaptures)

	// Inference defaults
	v.SetDefault("inference.enabled", defaults.Inference.Enabled)
	v.SetDefault("inference.min_samples", defaults.Inference.MinSamples)
	v.SetDefault("inference.max_enum_values", defaults.Inference.MaxEnumValues)
	v.SetDefault("inference.enum_threshold", defaults.Inference.EnumThreshold)
	v.SetDefault("inference.infer_on_capture", defaults.Inference.InferOnCapture)

	// LLM defaults
	v.SetDefault("llm.provider", defaults.LLM.Provider)
	v.SetDefault("llm.base_url", defaults.LLM.BaseURL)
	v.SetDefault("llm.model", defaults.LLM.Model)
	v.SetDefault("llm.max_tokens", defaults.LLM.MaxTokens)
	v.SetDefault("llm.temperature", defaults.LLM.Temperature)
	// API key read from env: AIPROXY_LLM_API_KEY or ANTHROPIC_API_KEY

	// MCP defaults
	v.SetDefault("mcp.db_path", defaults.MCP.DBPath)

	// Logging defaults
	v.SetDefault("logging.level", defaults.Logging.Level)
	v.SetDefault("logging.format", defaults.Logging.Format)
}

// InitLogger initializes the logger based on configuration
func InitLogger(cfg *models.LoggingConfig) (*zap.Logger, error) {
	var zapCfg zap.Config

	if cfg.Format == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		zapCfg.Level.SetLevel(zap.DebugLevel)
	case "info":
		zapCfg.Level.SetLevel(zap.InfoLevel)
	case "warn", "warning":
		zapCfg.Level.SetLevel(zap.WarnLevel)
	case "error":
		zapCfg.Level.SetLevel(zap.ErrorLevel)
	default:
		zapCfg.Level.SetLevel(zap.InfoLevel)
	}

	return zapCfg.Build()
}
