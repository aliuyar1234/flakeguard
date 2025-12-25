package auth

import (
	"golang.org/x/crypto/bcrypt"
)

const (
	// BcryptCost is the computational cost for password hashing
	// Cost 12 provides strong security while remaining performant
	BcryptCost = 12
)

// HashPassword hashes a plaintext password using bcrypt with cost 12
// Returns the bcrypt hash string or an error if hashing fails
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword compares a plaintext password against a bcrypt hash
// Returns nil if the password matches, an error otherwise
func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
