package api

import (
	_ "embed"
	"net/http"

	_ "kncc/internal/api/docs"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

//go:embed docs/swagger.json
var swaggerJSON []byte

//go:embed docs/swagger.yaml
var swaggerYAML []byte

// GetOpenAPIJSON serves the raw OpenAPI JSON specification.
func (h *Handler) GetOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(swaggerJSON)
}

// GetOpenAPIYAML serves the raw OpenAPI YAML specification.
func (h *Handler) GetOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(swaggerYAML)
}

// DocsHandler returns the http.HandlerFunc for Swagger UI.
func (h *Handler) DocsHandler() http.HandlerFunc {
	swaggerHandler := httpSwagger.Handler(
		httpSwagger.URL("/api/openapi.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/api/docs" || path == "/api/docs/" {
			http.Redirect(w, r, "/api/docs/index.html", http.StatusMovedPermanently)
			return
		}
		swaggerHandler.ServeHTTP(w, r)
	}
}
