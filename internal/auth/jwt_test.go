package auth

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateToken_AndValidateToken(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, err := CreateToken(userID, secret, 7)
	require.NoError(t, err)

	claims, err := ValidateToken(token, secret)
	require.NoError(t, err)
	require.Equal(t, userID, claims.UserID)
	require.Equal(t, userID.String(), claims.Subject)
	require.NotNil(t, claims.ExpiresAt)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	userID := uuid.New()
	token, err := CreateToken(userID, "secret-a", 7)
	require.NoError(t, err)

	_, err = ValidateToken(token, "secret-b")
	require.Error(t, err)
}

func TestValidateToken_Expired(t *testing.T) {
	userID := uuid.New()
	token, err := CreateToken(userID, "secret", -1)
	require.NoError(t, err)

	_, err = ValidateToken(token, "secret")
	require.Error(t, err)
}
