package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the broker configuration
type Config struct {
	// Basic auth credentials for the broker API
	Auth AuthConfig `yaml:"auth"`

	// Catalog configuration
	Catalog CatalogConfig `yaml:"catalog"`

	// ContextForge configuration
	ContextForge ContextForgeConfig `yaml:"contextforge"`

	// State store configuration
	StateStore StateStoreConfig `yaml:"state_store"`
}

// AuthConfig holds authentication credentials
type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// CatalogConfig holds service catalog configuration
type CatalogConfig struct {
	Services []ServiceConfig `yaml:"services"`
}

// ServiceConfig represents a service in the catalog
type ServiceConfig struct {
	ID          string          `yaml:"id"`
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Bindable    bool            `yaml:"bindable"`
	Tags        []string        `yaml:"tags"`
	Metadata    ServiceMetadata `yaml:"metadata"`
	Plans       []PlanConfig    `yaml:"plans"`
}

// ServiceMetadata holds service-level metadata
type ServiceMetadata struct {
	DisplayName         string `yaml:"displayName"`
	ImageURL            string `yaml:"imageUrl"`
	LongDescription     string `yaml:"longDescription"`
	ProviderDisplayName string `yaml:"providerDisplayName"`
	DocumentationURL    string `yaml:"documentationUrl"`
	SupportURL          string `yaml:"supportUrl"`
}

// PlanConfig represents a service plan
type PlanConfig struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Free        bool         `yaml:"free"`
	Metadata    PlanMetadata `yaml:"metadata"`
}

// PlanMetadata holds plan metadata including bullets
type PlanMetadata struct {
	DisplayName string   `yaml:"displayName"`
	Bullets     []string `yaml:"bullets"`
}

// ContextForgeConfig holds ContextForge gateway configuration
type ContextForgeConfig struct {
	URL            string `yaml:"url"`
	MCPURL         string `yaml:"mcp_url"`
	AdminUser      string `yaml:"admin_user"`
	AdminPassword  string `yaml:"admin_password"`
	JWTSecretKey   string `yaml:"jwt_secret_key"`
	JWTExpiryHours int    `yaml:"jwt_expiry_hours"`
}

// StateStoreConfig holds configuration for the state store
type StateStoreConfig struct {
	Path string `yaml:"path"`
}

// Load loads configuration from a YAML file and applies env var overrides
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Apply env var overrides
	if v := os.Getenv("BROKER_USERNAME"); v != "" {
		cfg.Auth.Username = v
	}
	if v := os.Getenv("BROKER_PASSWORD"); v != "" {
		cfg.Auth.Password = v
	}
	if v := os.Getenv("CONTEXTFORGE_URL"); v != "" {
		cfg.ContextForge.URL = v
	}
	if v := os.Getenv("CONTEXTFORGE_MCP_URL"); v != "" {
		cfg.ContextForge.MCPURL = v
	}
	if v := os.Getenv("CONTEXTFORGE_ADMIN_USER"); v != "" {
		cfg.ContextForge.AdminUser = v
	}
	if v := os.Getenv("CONTEXTFORGE_ADMIN_PASSWORD"); v != "" {
		cfg.ContextForge.AdminPassword = v
	}
	if v := os.Getenv("CONTEXTFORGE_JWT_SECRET_KEY"); v != "" {
		cfg.ContextForge.JWTSecretKey = v
	}

	// Set defaults
	if cfg.StateStore.Path == "" {
		cfg.StateStore.Path = "/home/vcap/app/state/broker-state.json"
	}
	if cfg.ContextForge.MCPURL == "" && cfg.ContextForge.URL != "" {
		cfg.ContextForge.MCPURL = cfg.ContextForge.URL + "/mcp"
	}

	return cfg, nil
}
