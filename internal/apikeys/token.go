package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const (
	// TokenPrefix is the prefix for all FlakeGuard API keys
	TokenPrefix = "fgk_"

	// TokenBytes is the number of random bytes in a token
	TokenBytes = 32
)

// GenerateToken creates a new API key token with the format: fgk_<base64url>
// Returns the plaintext token (to be shown once) and its SHA256 hash (for storage)
func GenerateToken() (token string, hash []byte, err error) {
	// Generate 32 random bytes
	randomBytes := make([]byte, TokenBytes)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode as base64url (URL-safe, no padding)
	encoded := base64.RawURLEncoding.EncodeToString(randomBytes)

	// Add prefix
	token = TokenPrefix + encoded

	// Generate SHA256 hash for storage
	hash = HashToken(token)

	return token, hash, nil
}

// HashToken computes the SHA256 hash of a token for storage
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// ValidateTokenFormat checks if a token has the correct format
func ValidateTokenFormat(token string) bool {
	if len(token) < len(TokenPrefix) {
		return false
	}

	// Check prefix
	if token[:len(TokenPrefix)] != TokenPrefix {
		return false
	}

	// Check that the remainder is valid base64url
	encoded := token[len(TokenPrefix):]
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}

	// Check that we got the expected number of bytes
	return len(decoded) == TokenBytes
}
