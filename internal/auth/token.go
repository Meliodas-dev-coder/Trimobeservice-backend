package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the custom JWT claims carried by an access token.
type Claims struct {
	Role Role `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager mints and verifies access tokens and mints refresh tokens.
type TokenManager struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(secret, issuer string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{
		secret:     []byte(secret),
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// GenerateAccess returns a signed HS256 access token and its expiry.
func (t *TokenManager) GenerateAccess(userID int64, role Role) (token string, expiresAt time.Time, err error) {
	now := time.Now()
	expiresAt = now.Add(t.accessTTL)
	claims := Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    t.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	return token, expiresAt, err
}

// ParseAccess validates a token string and returns its claims.
func (t *TokenManager) ParseAccess(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return t.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(t.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// newRefreshToken returns an opaque random token plus its storage hash. Only the
// hash is ever persisted.
func newRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
