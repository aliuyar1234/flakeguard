package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flakeguard/flakeguard/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	postgresContainerID string
	postgresHostPort    int
	postgresAdminPool   *pgxpool.Pool

	postgresUnavailableErr error
)

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err != nil {
		postgresUnavailableErr = err
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	containerID, hostPort, err := startPostgresContainer(ctx)
	if err != nil {
		postgresUnavailableErr = err
		os.Exit(m.Run())
	}

	postgresContainerID = containerID
	postgresHostPort = hostPort

	adminDSN := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", postgresHostPort)
	pool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		postgresUnavailableErr = err
		_ = stopContainer(context.Background(), postgresContainerID)
		os.Exit(m.Run())
	}

	if err := waitForPostgres(ctx, pool); err != nil {
		postgresUnavailableErr = err
		pool.Close()
		_ = stopContainer(context.Background(), postgresContainerID)
		os.Exit(m.Run())
	}

	postgresAdminPool = pool

	code := m.Run()

	postgresAdminPool.Close()
	_ = stopContainer(context.Background(), postgresContainerID)
	os.Exit(code)
}

func requirePostgres(t *testing.T) {
	t.Helper()
	if postgresAdminPool == nil || postgresUnavailableErr != nil {
		if postgresUnavailableErr == nil {
			postgresUnavailableErr = errors.New("postgres test container unavailable")
		}
		t.Skipf("skipping integration tests: %v", postgresUnavailableErr)
	}
}

func junitFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to locate test file")
	}

	return filepath.Join(filepath.Dir(currentFile), "..", "..", "testdata", "junit", name)
}

func startPostgresContainer(ctx context.Context) (containerID string, hostPort int, err error) {
	cmd := exec.CommandContext(ctx,
		"docker", "run",
		"-d",
		"--rm",
		"-e", "POSTGRES_PASSWORD=postgres",
		"-e", "POSTGRES_USER=postgres",
		"-e", "POSTGRES_DB=postgres",
		"-p", "127.0.0.1::5432",
		"postgres:16.4-alpine",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", 0, fmt.Errorf("failed to start postgres container: %w: %s", err, strings.TrimSpace(string(out)))
	}

	containerID = strings.TrimSpace(string(out))
	if containerID == "" {
		return "", 0, errors.New("docker run returned empty container id")
	}

	portCmd := exec.CommandContext(ctx, "docker", "port", containerID, "5432/tcp")
	portOut, err := portCmd.CombinedOutput()
	if err != nil {
		_ = stopContainer(context.Background(), containerID)
		return "", 0, fmt.Errorf("failed to resolve postgres port mapping: %w: %s", err, strings.TrimSpace(string(portOut)))
	}

	line := strings.TrimSpace(string(portOut))
	if idx := strings.LastIndex(line, "->"); idx != -1 {
		line = strings.TrimSpace(line[idx+2:])
	}
	colon := strings.LastIndex(line, ":")
	if colon == -1 {
		_ = stopContainer(context.Background(), containerID)
		return "", 0, fmt.Errorf("unexpected docker port output: %q", strings.TrimSpace(string(portOut)))
	}

	portStr := strings.TrimSpace(line[colon+1:])
	hostPort, err = strconv.Atoi(portStr)
	if err != nil {
		_ = stopContainer(context.Background(), containerID)
		return "", 0, fmt.Errorf("failed to parse docker port output %q: %w", strings.TrimSpace(string(portOut)), err)
	}

	return containerID, hostPort, nil
}

func stopContainer(ctx context.Context, containerID string) error {
	if strings.TrimSpace(containerID) == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
	_ = cmd.Run()
	return nil
}

func waitForPostgres(ctx context.Context, pool *pgxpool.Pool) error {
	deadline := time.Now().Add(45 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("postgres not ready: %w", lastErr)
}

func newTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	requirePostgres(t)

	dbName := "flakeguard_test_" + randomHex(t, 8)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := postgresAdminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		t.Fatalf("failed to create database %q: %v", dbName, err)
	}

	dsn := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", postgresHostPort, dbName)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		_, _ = postgresAdminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		t.Fatalf("failed to connect to database %q: %v", dbName, err)
	}

	if err := db.RunMigrations(ctx, pool); err != nil {
		pool.Close()
		_, _ = postgresAdminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		t.Fatalf("failed to run migrations: %v", err)
	}

	cleanup := func() {
		pool.Close()

		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, _ = postgresAdminPool.Exec(cleanupCtx, `
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()
		`, dbName)
		_, _ = postgresAdminPool.Exec(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	}

	return pool, cleanup
}

func randomHex(t *testing.T, bytes int) string {
	t.Helper()
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("failed to generate random bytes: %v", err)
	}
	return hex.EncodeToString(b)
}
