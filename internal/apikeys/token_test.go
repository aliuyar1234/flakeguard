package apikeys

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateToken_AndValidateFormatAndHash(t *testing.T) {
	token, hash, err := GenerateToken()
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(token, TokenPrefix))
	require.True(t, ValidateTokenFormat(token))
	require.Len(t, hash, sha256.Size)
	require.Equal(t, HashToken(token), hash)
}

func TestValidateTokenFormat_InvalidPrefix(t *testing.T) {
	require.False(t, ValidateTokenFormat("nope_abc"))
}
