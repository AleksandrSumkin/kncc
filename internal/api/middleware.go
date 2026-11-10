package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"kncc/internal/auth"
	"kncc/internal/model"
	"kncc/internal/ratelimit"
)

type contextKey string

const userClaimsKey contextKey = "userClaims"

// UserFromContext retrieves UserClaims from the HTTP request context.
func UserFromContext(ctx context.Context) *auth.UserClaims {
	claims, ok := ctx.Value(userClaimsKey).(*auth.UserClaims)
	if !ok {
		return nil
	}
	return claims
}

// SecurityHeadersMiddleware attaches standard HTTP security headers to all responses.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// For Swagger UI documentation (/api/docs), allow 'unsafe-inline' for scripts
		// because Swagger UI renders an inline initialization script in index.html.
		if strings.HasPrefix(r.URL.Path, "/api/docs") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		}

		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware handles CORS headers for cross-origin requests and preflight OPTIONS.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			if origin == "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// AuthMiddleware extracts JWT token from Authorization header or cookie.
func AuthMiddleware(authSvc *auth.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""

			// 1. Check Authorization Bearer header
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenString = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// 2. Check kncc_token cookie
			if tokenString == "" {
				if cookie, err := r.Cookie("kncc_token"); err == nil {
					tokenString = cookie.Value
				}
			}

			if tokenString != "" {
				claims, err := authSvc.ValidateJWT(tokenString)
				if err == nil && claims != nil {
					ctx := context.WithValue(r.Context(), userClaimsKey, claims)
					r = r.WithContext(ctx)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth blocks unauthenticated requests with HTTP 401 Unauthorized.
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := UserFromContext(r.Context())
		if claims == nil {
			writeJSONError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// RequireAdmin blocks non-admin requests with HTTP 403 Forbidden.
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := UserFromContext(r.Context())
		if claims == nil {
			writeJSONError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		if claims.Role != model.RoleAdmin {
			writeJSONError(w, http.StatusForbidden, "Admin role required")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// RateLimitMiddleware enforces request rate limits per user ID or IP.
func RateLimitMiddleware(limiter *ratelimit.Limiter) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr
			claims := UserFromContext(r.Context())
			if claims != nil {
				key = claims.UserID
			}

			if !limiter.Allow(key) {
				writeJSONError(w, http.StatusTooManyRequests, "Rate limit exceeded. Please wait before submitting more checks.")
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}
