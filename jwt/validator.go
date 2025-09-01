package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// Config holds JWT validation configuration
type Config struct {
	// Enable JWT validation
	Enabled bool
	// Path to RSA public key file for JWT validation
	RSAKeyFile string
	// Header name to extract JWT from (defaults to "Authorization")
	Header string
	// Bearer token prefix (defaults to "Bearer ")
	TokenPrefix string
}

// Validator handles JWT validation with RSA256
type Validator struct {
	publicKey   *rsa.PublicKey
	header      string
	tokenPrefix string
}

// NewValidator creates a new JWT validator with RSA256 support
func NewValidator(config Config) (*Validator, error) {
	if !config.Enabled {
		return nil, nil
	}

	if config.RSAKeyFile == "" {
		return nil, fmt.Errorf("RSA key file path is required when JWT is enabled")
	}

	// Read and parse RSA public key
	keyData, err := os.ReadFile(config.RSAKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read RSA key file: %w", err)
	}

	publicKey, err := parseRSAPublicKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA public key: %w", err)
	}

	header := config.Header
	if header == "" {
		header = "Authorization"
	}

	tokenPrefix := config.TokenPrefix
	if tokenPrefix == "" {
		tokenPrefix = "Bearer "
	}

	slog.Info("JWT validator initialized", "rsa_key_file", config.RSAKeyFile, "header", header)

	return &Validator{
		publicKey:   publicKey,
		header:      header,
		tokenPrefix: tokenPrefix,
	}, nil
}

// ValidateRequest validates the JWT token from the HTTP request
func (v *Validator) ValidateRequest(req *http.Request) (*gojwt.Token, error) {
	if v == nil {
		// JWT validation is disabled
		return nil, nil
	}

	// Extract token from header
	tokenString := v.extractToken(req)
	if tokenString == "" {
		return nil, fmt.Errorf("JWT token not found in %s header", v.header)
	}

	// Parse and validate the token
	token, err := gojwt.Parse(tokenString, func(token *gojwt.Token) (interface{}, error) {
		// Verify the signing method is RSA256
		if _, ok := token.Method.(*gojwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to validate JWT: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid JWT token")
	}

	return token, nil
}

// extractToken extracts the JWT token from the request header
func (v *Validator) extractToken(req *http.Request) string {
	authHeader := req.Header.Get(v.header)
	if authHeader == "" {
		return ""
	}

	if strings.HasPrefix(authHeader, v.tokenPrefix) {
		return strings.TrimPrefix(authHeader, v.tokenPrefix)
	}

	// If no prefix is configured, return the entire header value
	if v.tokenPrefix == "" {
		return authHeader
	}

	return ""
}

// parseRSAPublicKey parses an RSA public key from PEM data
func parseRSAPublicKey(keyData []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing public key")
	}

	// Try parsing as PKCS#1 RSA public key first
	if block.Type == "RSA PUBLIC KEY" {
		return x509.ParsePKCS1PublicKey(block.Bytes)
	}

	// Try parsing as PKIX public key (more common format)
	if block.Type == "PUBLIC KEY" {
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}

		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("key is not an RSA public key")
		}
		return rsaPub, nil
	}

	return nil, fmt.Errorf("unsupported key type: %s", block.Type)
}
