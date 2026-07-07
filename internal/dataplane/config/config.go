package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds data-plane configuration, loaded from environment variables
// with the SHEELD_DP_ prefix.
type Config struct {
	// Server
	Port            int           `envconfig:"PORT" default:"8081"`
	ReadTimeout     time.Duration `envconfig:"READ_TIMEOUT" default:"30s"`
	WriteTimeout    time.Duration `envconfig:"WRITE_TIMEOUT" default:"60s"`
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`

	// Control plane
	ControlPlaneURL string        `envconfig:"CONTROL_PLANE_URL" required:"true"`
	Token           string        `envconfig:"TOKEN" required:"true"`
	PollInterval    time.Duration `envconfig:"POLL_INTERVAL" default:"5s"`
	StartupTimeout  time.Duration `envconfig:"STARTUP_TIMEOUT" default:"60s"`
	// The workspace-config payload carries plaintext LLM keys, so the
	// control-plane URL must be HTTPS unless explicitly overridden for
	// local development.
	AllowInsecureCP bool `envconfig:"ALLOW_INSECURE_CP" default:"false"`

	// AllowPrivateGuardURLs permits guard/transformer URLs resolving to
	// private/loopback/link-local addresses (SSRF protection is off).
	// Enable inside the compose stack where presidio/webhook targets run on
	// the internal network. Applies when the data plane builds guards.
	AllowPrivateGuardURLs bool `envconfig:"ALLOW_PRIVATE_GUARD_URLS" default:"false"`

	// Config snapshot: when both are set, each applied workspace config is
	// persisted to disk (AES-256-GCM encrypted, 0600) and used as a startup
	// fallback if the control plane is unreachable. The payload contains
	// plaintext LLM keys, so the key is mandatory — never stored plaintext.
	ConfigSnapshotPath string `envconfig:"CONFIG_SNAPSHOT_PATH" default:""`
	ConfigSnapshotKey  string `envconfig:"CONFIG_SNAPSHOT_KEY" default:""`

	// Database (audit logs)
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// LLM Gateway (LiteLLM)
	LLMGatewayURL     string        `envconfig:"LLM_GATEWAY_URL" default:"http://localhost:4000"`
	LLMRequestTimeout time.Duration `envconfig:"LLM_REQUEST_TIMEOUT" default:"30s"`
	// Retries after the first attempt for transient gateway failures
	// (connection errors, HTTP 429/5xx), with exponential backoff.
	LLMMaxRetries   int           `envconfig:"LLM_MAX_RETRIES" default:"2"`
	LLMRetryBackoff time.Duration `envconfig:"LLM_RETRY_BACKOFF" default:"200ms"`

	// Rate Limiting
	RateLimitRPS   float64 `envconfig:"RATE_LIMIT_RPS" default:"100"`
	RateLimitBurst int     `envconfig:"RATE_LIMIT_BURST" default:"200"`

	// Request Body Limit
	MaxBodyBytes int64 `envconfig:"MAX_BODY_BYTES" default:"1048576"`

	// Proxy Timeout
	ProxyTimeout time.Duration `envconfig:"PROXY_TIMEOUT" default:"60s"`

	// Logging
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
}

// Load reads configuration from environment variables with the SHEELD_DP_ prefix.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("sheeld_dp", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if !strings.HasPrefix(cfg.ControlPlaneURL, "https://") && !cfg.AllowInsecureCP {
		return nil, fmt.Errorf("SHEELD_DP_CONTROL_PLANE_URL must use https (the config payload carries secrets); set SHEELD_DP_ALLOW_INSECURE_CP=true only for local development")
	}
	return &cfg, nil
}
