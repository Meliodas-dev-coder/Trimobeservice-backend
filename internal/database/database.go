// Package database owns the MySQL connection pool and schema migrations.
package database

import (
	"time"

	_ "github.com/go-sql-driver/mysql" // registers the "mysql" driver
	"github.com/jmoiron/sqlx"
)

// Connect opens (and verifies) a MySQL connection pool.
func Connect(dsn string, maxOpen, maxIdle int, connMaxLifetime time.Duration) (*sqlx.DB, error) {
	db, err := sqlx.Connect("mysql", dsn) // Connect pings to verify the connection
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(connMaxLifetime)
	return db, nil
}
