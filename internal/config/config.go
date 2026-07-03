// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Config is the fully resolved application configuration.
type Config struct {
	Env      string // "development" | "production"
	LogLevel string // "debug" | "info" | "warn" | "error"

	HTTP    HTTPConfig
	DB      DBConfig
	JWT     JWTConfig
	Storage StorageConfig
}

type HTTPConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	CORSOrigins     []string
}

type DBConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type JWTConfig struct {
	Secret     string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// StorageConfig configures object storage for uploaded images (Firebase/GCS).
// When Bucket is empty, uploads are disabled.
type StorageConfig struct {
	Bucket          string // GCS/Firebase bucket, e.g. my-project.appspot.com
	CredentialsFile string // path to a service-account JSON (optional; falls back to ADC)
	PublicBaseURL   string // base for public object URLs
	MaxUploadBytes  int64
}

// Load reads configuration from the environment, applying sensible defaults.
// It returns an error for required-but-missing values (currently JWT_SECRET).
func Load() (*Config, error) {
	cfg := &Config{
		Env:      getStr("APP_ENV", "development"),
		LogLevel: getStr("LOG_LEVEL", "info"),
		HTTP: HTTPConfig{
			Port:            getStr("HTTP_PORT", "8080"),
			ReadTimeout:     getDur("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getDur("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getDur("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getDur("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
			CORSOrigins:     getList("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://localhost:5174"}),
		},
		DB: DBConfig{
			Host:            getStr("DB_HOST", "127.0.0.1"),
			Port:            getStr("DB_PORT", "3306"),
			User:            getStr("DB_USER", "root"),
			Password:        getStr("DB_PASSWORD", "123456789"),
			Name:            getStr("DB_NAME", "trimobase"),
			MaxOpenConns:    getInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getInt("DB_MAX_IDLE_CONNS", 25),
			ConnMaxLifetime: getDur("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		JWT: JWTConfig{
			Secret:     getStr("JWT_SECRET", "321sd32f13d1f3sd1f2s1f2s31df23s1f1dss"),
			Issuer:     getStr("JWT_ISSUER", "trimo"),
			AccessTTL:  getDur("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL: getDur("JWT_REFRESH_TTL", 720*time.Hour), // 30 days
		},
		Storage: StorageConfig{
			Bucket:          getStr("STORAGE_BUCKET", ""),
			CredentialsFile: getStr("GOOGLE_APPLICATION_CREDENTIALS", ""),
			PublicBaseURL:   getStr("STORAGE_PUBLIC_BASE_URL", "https://storage.googleapis.com"),
			MaxUploadBytes:  int64(getInt("STORAGE_MAX_UPLOAD_BYTES", 5*1024*1024)),
		},
	}

	if cfg.JWT.Secret == "" {
		return nil, fmt.Errorf("config: JWT_SECRET is required")
	}
	return cfg, nil
}

// DSN builds the base MySQL DSN used by the application connection pool.
// It enables ParseTime and uses utf8mb4; it deliberately does NOT enable
// multiStatements (migrations use their own connection for that).
func (c DBConfig) DSN() string {
	m := mysql.NewConfig()
	m.User = c.User
	m.Passwd = c.Password
	m.Net = "tcp"
	m.Addr = net.JoinHostPort(c.Host, c.Port)
	m.DBName = c.Name
	m.Collation = "utf8mb4_unicode_ci"
	m.ParseTime = true
	m.Loc = time.UTC
	return m.FormatDSN()
}

func getStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getList(key string, def []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
