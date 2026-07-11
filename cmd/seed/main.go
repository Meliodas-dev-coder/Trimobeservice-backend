// Command seed creates a super-admin user (or promotes an existing user to
// admin). The public API never grants the admin role, so this is how the first
// admin is bootstrapped.
//
//	go run ./cmd/seed -email=admin@trimo.dev -password='a-long-random-passphrase' -name="Super Admin"
//
// No credentials are baked in: -email and -password must be supplied explicitly.
//
// If a user with the email already exists, it is promoted to admin (and its
// password reset only if -password is supplied). Migrations are applied first,
// so this works against a fresh database.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"github.com/trimo/backend/internal/config"
	"github.com/trimo/backend/internal/database"
	"github.com/trimo/backend/migrations"
)

func main() {
	email := flag.String("email", "", "admin email (required)")
	password := flag.String("password", "", "admin password, min 12 chars (required to create; optional to reset)")
	name := flag.String("name", "Administrator", "admin full name")
	flag.Parse()

	if err := run(*email, *password, *name); err != nil {
		log.Fatalf("seed admin: %v", err)
	}
}

func run(email, password, name string) error {
	_ = godotenv.Load()

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("-email is required")
	}
	if password != "" && len(password) < 12 {
		return errors.New("-password must be at least 12 characters")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Ensure the schema exists, then open a small pool.
	if err := database.Migrate(cfg.DB.DSN(), migrations.FS); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	db, err := database.Connect(cfg.DB.DSN(), 2, 1, 5*time.Minute)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var existingID int64
	err = db.GetContext(ctx, &existingID, `SELECT id FROM users WHERE email = ?`, email)
	switch {
	case err == nil:
		if password != "" {
			hash, herr := hashPassword(password)
			if herr != nil {
				return herr
			}
			if _, uerr := db.ExecContext(ctx,
				`UPDATE users SET role = 'admin', is_active = TRUE, password_hash = ? WHERE id = ?`,
				hash, existingID); uerr != nil {
				return uerr
			}
			fmt.Printf("Promoted %q to admin and reset its password (id=%d)\n", email, existingID)
			return nil
		}
		if _, uerr := db.ExecContext(ctx,
			`UPDATE users SET role = 'admin', is_active = TRUE WHERE id = ?`, existingID); uerr != nil {
			return uerr
		}
		fmt.Printf("Promoted existing user %q to admin (id=%d)\n", email, existingID)
		return nil

	case errors.Is(err, sql.ErrNoRows):
		if len(password) < 12 {
			return errors.New("-password (min 12 chars) is required to create a new admin")
		}
		fullName := strings.TrimSpace(name)
		if fullName == "" {
			fullName = "Administrator"
		}
		hash, herr := hashPassword(password)
		if herr != nil {
			return herr
		}
		res, ierr := db.ExecContext(ctx,
			`INSERT INTO users (role, email, password_hash, full_name, is_active)
			 VALUES ('admin', ?, ?, ?, TRUE)`,
			email, hash, fullName)
		if ierr != nil {
			return ierr
		}
		id, _ := res.LastInsertId()
		fmt.Printf("Created admin %q (id=%d)\n", email, id)
		return nil

	default:
		return err
	}
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}
