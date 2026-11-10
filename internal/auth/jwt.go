package auth

import (
	"errors"
	"fmt"
	"time"

	"kncc/internal/model"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
)

// UserClaims represents the JWT claims payload.
type UserClaims struct {
	UserID   string     `json:"user_id"`
	Username string     `json:"username"`
	Role     model.Role `json:"role"`
	Provider string     `json:"provider"`
	jwt.RegisteredClaims
}

// JWTManager handles signing and validating JWT tokens.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTManager creates a new JWTManager.
func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &JWTManager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// GenerateToken signs a new JWT token for the user.
func (j *JWTManager) GenerateToken(u *model.User) (string, error) {
	now := time.Now().UTC()
	claims := UserClaims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		Provider: u.Provider,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.ttl)),
			Issuer:    "kncc",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken verifies and parses the JWT token string.
func (j *JWTManager) ValidateToken(tokenString string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
