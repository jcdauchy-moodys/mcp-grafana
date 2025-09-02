package jwt

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"gopkg.in/yaml.v3"
)

// Claims represents the expected JWT payload structure
type Claims struct {
	Sub      string                 `json:"sub"`
	Cluster  string                 `json:"cluster"`
	Username string                 `json:"username"`
	UserRole string                 `json:"userrole"`
	Exp      int64                  `json:"exp"`
	Kid      string                 `json:"kid"`
	Scope    map[string]interface{} `json:"scope"`
}

// GetExpirationTime implements gojwt.Claims interface
func (c *Claims) GetExpirationTime() (*gojwt.NumericDate, error) {
	if c.Exp == 0 {
		return nil, nil
	}
	return gojwt.NewNumericDate(time.Unix(c.Exp, 0)), nil
}

// GetNotBefore implements gojwt.Claims interface
func (c *Claims) GetNotBefore() (*gojwt.NumericDate, error) {
	return nil, nil
}

// GetIssuedAt implements gojwt.Claims interface
func (c *Claims) GetIssuedAt() (*gojwt.NumericDate, error) {
	return nil, nil
}

// GetAudience implements gojwt.Claims interface
func (c *Claims) GetAudience() (gojwt.ClaimStrings, error) {
	return nil, nil
}

// GetIssuer implements gojwt.Claims interface
func (c *Claims) GetIssuer() (string, error) {
	return "", nil
}

// GetSubject implements gojwt.Claims interface
func (c *Claims) GetSubject() (string, error) {
	return c.Sub, nil
}

// RolePermissions represents the tools allowed for a specific user role
type RolePermissions struct {
	// List of tool names allowed for this role. Use "*" for all tools.
	Tools []string `yaml:"tools"`
}

// YAMLConfig represents the JWT configuration loaded from YAML
type YAMLConfig struct {
	// Enable JWT validation
	Enabled bool `yaml:"enabled"`
	
	// Expected cluster name
	ClusterName string `yaml:"cluster_name"`
	
	// List of allowed usernames
	AllowedUsers []string `yaml:"allowed_users"`
	
	// Role-based tool permissions
	RolePermissions map[string]RolePermissions `yaml:"role_permissions,omitempty"`
	
	// RSA public key for JWT validation (inline PEM format)
	PublicKey string `yaml:"publicKey"`
	
	// HTTP header name to extract JWT from (defaults to "Authorization")
	Header string `yaml:"header,omitempty"`
	
	// Bearer token prefix (defaults to "Bearer ")
	TokenPrefix string `yaml:"token_prefix,omitempty"`
}

// LoadYAMLConfig loads JWT configuration from a YAML file
func LoadYAMLConfig(configPath string) (*YAMLConfig, error) {
	if configPath == "" {
		return &YAMLConfig{Enabled: false}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWT config file %s: %w", configPath, err)
	}

	slog.Info("Successfully read JWT config file", "config_path", configPath)

	var config YAMLConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JWT config YAML: %w", err)
	}

	// Set defaults
	if config.Header == "" {
		config.Header = "Authorization"
	}
	if config.TokenPrefix == "" {
		config.TokenPrefix = "Bearer "
	}

	// Validate configuration
	if config.Enabled {
		if config.ClusterName == "" {
			return nil, fmt.Errorf("cluster_name is required when JWT is enabled")
		}
		if len(config.AllowedUsers) == 0 {
			return nil, fmt.Errorf("at least one allowed user is required when JWT is enabled")
		}
		if config.PublicKey == "" {
			return nil, fmt.Errorf("publicKey is required when JWT is enabled")
		}
	}

	return &config, nil
}

// GetAllowedToolsForRole returns the list of tools allowed for a specific user role
func (yc *YAMLConfig) GetAllowedToolsForRole(role string) []string {
	if yc.RolePermissions == nil {
		// No role permissions configured, allow all tools
		return []string{"*"}
	}
	
	if permissions, exists := yc.RolePermissions[role]; exists {
		return permissions.Tools
	}
	
	// Role not found, return empty list (no tools allowed)
	return []string{}
}

// ToConfig converts YAMLConfig to the original Config format for backward compatibility
func (yc *YAMLConfig) ToConfig() Config {
	return Config{
		Enabled:     yc.Enabled,
		RSAKeyFile:  "", // Will be handled by YAMLValidator
		Header:      yc.Header,
		TokenPrefix: yc.TokenPrefix,
	}
}
