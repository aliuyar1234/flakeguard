package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegration_MigrationsApplyToFreshPostgres(t *testing.T) {
	pool, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	var count int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'users'
	`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
