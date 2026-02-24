package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	// Server
	Port            int           `envconfig:"PORT" default:"8080"`
	ReadTimeout     time.Duration `envconfig:"READ_TIMEOUT" default:"30s"`
	WriteTimeout    time.Duration `envconfig:"WRITE_TIMEOUT" default:"60s"`
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`

	// Database
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// Auth
	JWTSecret     string        `envconfig:"JWT_SECRET" required:"true"`
	JWTExpiration time.Duration `envconfig:"JWT_EXPIRATION" default:"24h"`

	// Encryption key for LLM API keys at rest (hex-encoded 32 bytes for AES-256)
	EncryptionKey string `envconfig:"ENCRYPTION_KEY" required:"true"`

	// LLM Gateway (LiteLLM)
	LLMGatewayURL     string        `envconfig:"LLM_GATEWAY_URL" default:"http://localhost:4000"`
	LLMRequestTimeout time.Duration `envconfig:"LLM_REQUEST_TIMEOUT" default:"30s"`

	// CORS
	CORSAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS" default:"http://localhost:3000"`

	// Logging
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
}

// Load reads configuration from environment variables with the SHEELD_ prefix.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("sheeld", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return &cfg, nil
}
