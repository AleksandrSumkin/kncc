package api

import (
	"io/fs"
	"net/http"
	"strings"

	"kncc/internal/auth"
	"kncc/internal/ratelimit"
)

// SetupRouter initializes and configures all API and static file routes.
func SetupRouter(h *Handler, authSvc *auth.AuthService, limiter *ratelimit.Limiter, webFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	// 1. Health & Lifecycle Probes (Kubernetes)
	mux.HandleFunc("GET /api/livez", h.Livez)
	mux.HandleFunc("GET /api/readyz", h.Readyz)
	mux.HandleFunc("GET /api/startupz", h.Startupz)
	mux.HandleFunc("GET /api/healthz", h.Healthz)

	// 2. Public System & Auth routes
	mux.HandleFunc("GET /api/system/config", h.GetSystemConfig)
	mux.HandleFunc("GET /api/system/logo", h.GetSystemLogo)
	mux.HandleFunc("GET /api/system/favicon", h.GetSystemFavicon)
	mux.HandleFunc("GET /favicon.ico", h.GetSystemFavicon)
	mux.HandleFunc("GET /api/auth/providers", h.GetAuthProviders)
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.HandleFunc("POST /api/auth/logout", h.Logout)
	mux.HandleFunc("GET /api/auth/oidc/login", h.OIDCLogin)
	mux.HandleFunc("GET /api/auth/oidc/callback", h.OIDCCallback)
	mux.HandleFunc("POST /api/auth/oidc/callback", h.OIDCCallback)

	// 3. Authenticated routes
	mux.HandleFunc("GET /api/auth/me", RequireAuth(h.GetMe))
	mux.HandleFunc("GET /api/k8s/namespaces", RequireAuth(h.GetNamespaces))
	mux.HandleFunc("GET /api/checks/quick-add", RequireAuth(h.GetQuickAddTargets))
	mux.HandleFunc("POST /api/checks", RequireAuth(RateLimitMiddleware(limiter)(h.CreateCheck)))
	mux.HandleFunc("GET /api/reports", RequireAuth(h.ListReports))
	mux.HandleFunc("GET /api/reports/{id}", RequireAuth(h.GetReport))
	mux.HandleFunc("POST /api/reports/{id}/rerun", RequireAuth(RateLimitMiddleware(limiter)(h.RerunCheck)))

	// 4. Admin routes
	mux.HandleFunc("GET /api/admin/users", RequireAdmin(h.ListUsers))
	mux.HandleFunc("POST /api/admin/users", RequireAdmin(h.CreateUser))
	mux.HandleFunc("GET /api/admin/stats/users", RequireAdmin(h.GetUserStats))
	mux.HandleFunc("GET /api/admin/rbac", RequireAdmin(h.GetRBACPermissions))
	mux.HandleFunc("PUT /api/admin/settings/theme", RequireAdmin(h.UpdateThemeSettings))
	mux.HandleFunc("POST /api/admin/settings/logo", RequireAdmin(h.UploadLogo))
	mux.HandleFunc("DELETE /api/admin/settings/logo", RequireAdmin(h.ResetLogo))
	mux.HandleFunc("POST /api/admin/settings/favicon", RequireAdmin(h.UploadFavicon))
	mux.HandleFunc("DELETE /api/admin/settings/favicon", RequireAdmin(h.ResetFavicon))

	// 5. OpenAPI & Documentation routes
	if h.cfg != nil && h.cfg.Server.OpenAPI.Enabled {
		openAPIAuth := func(next http.HandlerFunc) http.HandlerFunc {
			if h.cfg.Server.OpenAPI.Public {
				return next
			}
			return RequireAuth(next)
		}

		mux.HandleFunc("GET /api/openapi.json", openAPIAuth(h.GetOpenAPIJSON))
		mux.HandleFunc("GET /api/openapi.yaml", openAPIAuth(h.GetOpenAPIYAML))
		mux.HandleFunc("GET /api/docs", openAPIAuth(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/api/docs/index.html", http.StatusMovedPermanently)
		}))
		mux.HandleFunc("/api/docs/", openAPIAuth(h.DocsHandler()))
	}

	// 6. Embedded SPA / Static Frontend (if provided)
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// If request path starts with /api/, do not fallback to index.html
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}

			// Check if file exists in webFS
			filePath := strings.TrimPrefix(r.URL.Path, "/")
			if filePath == "" {
				filePath = "index.html"
			}
			f, err := webFS.Open(filePath)
			if err != nil {
				// Fallback to index.html for SPA routing
				r.URL.Path = "/"
			} else {
				_ = f.Close()
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	// Apply global middleware: Logging -> SecurityHeaders -> CORS -> Auth extraction
	var rootHandler http.Handler = mux
	rootHandler = AuthMiddleware(authSvc)(rootHandler)
	rootHandler = CORSMiddleware(rootHandler)
	rootHandler = SecurityHeadersMiddleware(rootHandler)
	rootHandler = LoggingMiddleware(rootHandler)

	return rootHandler
}
