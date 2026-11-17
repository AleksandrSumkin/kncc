package metrics

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"kncc/internal/config"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsAuthMiddleware creates HTTP middleware enforcing Basic or Bearer authentication if configured.
func MetricsAuthMiddleware(cfg *config.MetricsAuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg == nil || !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			authType := strings.ToLower(strings.TrimSpace(cfg.Type))
			if authType == "bearer" {
				authHeader := r.Header.Get("Authorization")
				if !strings.HasPrefix(authHeader, "Bearer ") {
					w.Header().Set("WWW-Authenticate", `Bearer realm="kncc-metrics"`)
					http.Error(w, "Unauthorized: Bearer token required", http.StatusUnauthorized)
					return
				}
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if subtle.ConstantTimeCompare([]byte(token), []byte(cfg.Token)) != 1 {
					w.Header().Set("WWW-Authenticate", `Bearer realm="kncc-metrics"`)
					http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Default: Basic Auth
			username, password, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="kncc-metrics"`)
				http.Error(w, "Unauthorized: Basic auth required", http.StatusUnauthorized)
				return
			}

			userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(cfg.Username)) == 1
			passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(cfg.Password)) == 1

			if !userMatch || !passMatch {
				w.Header().Set("WWW-Authenticate", `Basic realm="kncc-metrics"`)
				http.Error(w, "Unauthorized: invalid username or password", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// StartMetricsServer starts a dedicated HTTP server for serving Prometheus metrics.
func StartMetricsServer(ctx context.Context, cfg *config.MetricsConfig, reg *prometheus.Registry) *http.Server {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	path := cfg.Path
	if path == "" {
		path = "/metrics"
	}

	host := cfg.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := cfg.Port
	if port <= 0 {
		port = 9090
	}

	mux := http.NewServeMux()
	promHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})

	mux.Handle(path, MetricsAuthMiddleware(&cfg.Auth)(promHandler))

	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		log.Printf("Prometheus metrics server listening on http://%s%s (auth_enabled=%v)", addr, path, cfg.Auth.Enabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	return srv
}
