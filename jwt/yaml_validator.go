package jwt

import (
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// YAMLValidator handles JWT validation using YAML configuration
type YAMLValidator struct {
	config    *YAMLConfig
	publicKey *rsa.PublicKey
}

// NewYAMLValidator creates a new JWT validator from YAML configuration
func NewYAMLValidator(configPath string) (*YAMLValidator, error) {
	config, err := LoadYAMLConfig(configPath)
	if err != nil {
		return nil, err
	}

	if !config.Enabled {
		return nil, nil
	}

	// Parse the RSA public key
	keyData := []byte(config.PublicKey)
	publicKey, err := parseRSAPublicKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA public key: %w", err)
	}

	slog.Debug("Successfully parsed RSA public key from config file", "config_path", configPath)

	return &YAMLValidator{
		config:    config,
		publicKey: publicKey,
	}, nil
}

// ValidateRequest validates the JWT token from the HTTP request with username and cluster checks
func (v *YAMLValidator) ValidateRequest(req *http.Request) (*gojwt.Token, *Claims, error) {
	if v == nil {
		// JWT validation is disabled
		return nil, nil, nil
	}

	// Extract token from header
	tokenString := v.extractToken(req)
	if tokenString == "" {
		return nil, nil, fmt.Errorf("JWT token not found in %s header", v.config.Header)
	}

	// Parse and validate the token
	token, err := gojwt.ParseWithClaims(tokenString, &Claims{}, func(token *gojwt.Token) (interface{}, error) {
		// Verify the signing method is RSA256
		if _, ok := token.Method.(*gojwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Use the configured public key
		return v.publicKey, nil
	})

	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate JWT: %w", err)
	}

	if !token.Valid {
		return nil, nil, fmt.Errorf("invalid JWT token")
	}

	// Extract and validate claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, nil, fmt.Errorf("failed to parse JWT claims")
	}

	// Validate cluster name
	if claims.Cluster != v.config.ClusterName {
		return nil, nil, fmt.Errorf("cluster mismatch: expected %s, got %s", v.config.ClusterName, claims.Cluster)
	}

	// Validate username is in allowed list
	if !slices.Contains(v.config.AllowedUsers, claims.Username) {
		return nil, nil, fmt.Errorf("username %s is not in allowed users list", claims.Username)
	}

	slog.Debug("JWT validation successful",
		"username", claims.Username,
		"cluster", claims.Cluster,
		"role", claims.UserRole)

	return token, claims, nil
}

// extractToken extracts the JWT token from the request header
func (v *YAMLValidator) extractToken(req *http.Request) string {
	authHeader := req.Header.Get(v.config.Header)
	if authHeader == "" {
		return ""
	}

	if strings.HasPrefix(authHeader, v.config.TokenPrefix) {
		return strings.TrimPrefix(authHeader, v.config.TokenPrefix)
	}

	// If no prefix is configured, return the entire header value
	if v.config.TokenPrefix == "" {
		return authHeader
	}

	return ""
}
