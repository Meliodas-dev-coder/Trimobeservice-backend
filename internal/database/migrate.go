package database

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies all pending "up" migrations from the given filesystem.
//
// It opens a dedicated connection with multiStatements enabled — required so
// the multi-table migrations and the trigger migration (000007) execute as a
// single batch — then closes it. This keeps the application's main pool free of
// multiStatements.
func Migrate(baseDSN string, files fs.FS) error {
	cfg, err := mysql.ParseDSN(baseDSN)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MultiStatements = true

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer db.Close()

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}

	src, err := iofs.New(files, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
