package orgs

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const (
	InviteTokenPrefix = "fgi_"
	InviteTokenBytes  = 32
)

func GenerateInviteToken() (token string, hash []byte, err error) {
	randomBytes := make([]byte, InviteTokenBytes)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(randomBytes)
	token = InviteTokenPrefix + encoded
	hash = HashInviteToken(token)

	return token, hash, nil
}

func HashInviteToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func ValidateInviteTokenFormat(token string) bool {
	if len(token) < len(InviteTokenPrefix) {
		return false
	}

	if token[:len(InviteTokenPrefix)] != InviteTokenPrefix {
		return false
	}

	encoded := token[len(InviteTokenPrefix):]
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}

	return len(decoded) == InviteTokenBytes
}
