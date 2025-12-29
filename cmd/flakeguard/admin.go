package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aliuyar1234/flakeguard/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runAdmin(args []string) int {
	if len(args) == 0 {
		printAdminUsage()
		return 2
	}

	switch args[0] {
	case "reset-password":
		return runResetPassword(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown admin command: %s\n", args[0])
		printAdminUsage()
		return 2
	}
}

func printAdminUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  flakeguard admin reset-password --email user@example.com [--password <new>] [--db-dsn <dsn>]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Notes:")
	fmt.Fprintln(os.Stderr, "  - If --password is omitted, a random password is generated and printed.")
	fmt.Fprintln(os.Stderr, "  - --db-dsn defaults to FG_DB_DSN.")
}

func runResetPassword(args []string) int {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var email string
	var password string
	var dbDSN string

	fs.StringVar(&email, "email", "", "User email")
	fs.StringVar(&password, "password", "", "New password (if empty, generates one)")
	fs.StringVar(&dbDSN, "db-dsn", "", "Postgres DSN (defaults to FG_DB_DSN)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	email = strings.TrimSpace(email)
	if email == "" {
		fmt.Fprintln(os.Stderr, "--email is required")
		return 2
	}

	if dbDSN == "" {
		dbDSN = strings.TrimSpace(os.Getenv("FG_DB_DSN"))
	}
	if dbDSN == "" {
		fmt.Fprintln(os.Stderr, "--db-dsn is required (or set FG_DB_DSN)")
		return 2
	}

	generated := false
	if password == "" {
		pw, err := generatePassword(24)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate password: %v\n", err)
			return 1
		}
		password = pw
		generated = true
	}

	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "Password must be at least 8 characters")
		return 2
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash password: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		return 1
	}
	defer pool.Close()

	tag, err := pool.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE email = $1`, email, passwordHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update password: %v\n", err)
		return 1
	}
	if tag.RowsAffected() == 0 {
		fmt.Fprintf(os.Stderr, "No user found with email %q\n", email)
		return 1
	}

	fmt.Fprintln(os.Stdout, "Password updated.")
	if generated {
		fmt.Fprintln(os.Stdout, password)
	}

	return 0
}

func generatePassword(bytesLen int) (string, error) {
	if bytesLen < 8 {
		bytesLen = 8
	}

	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// URL-safe, printable, without padding.
	return base64.RawURLEncoding.EncodeToString(b), nil
}
