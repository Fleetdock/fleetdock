package httpapi

import (
	"net/http"

	"github.com/TajBrains/fleetdock/backend/internal/openapi"
)

// DocsHandler serves the OpenAPI spec and a self-contained Redoc docs page.
// Both routes are public (like /healthz) so the spec is easy to consume.
type DocsHandler struct{}

// NewDocsHandler builds the docs handler.
func NewDocsHandler() *DocsHandler { return &DocsHandler{} }

// Spec handles GET /openapi.yaml.
func (h *DocsHandler) Spec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapi.Spec)
}

const redocPage = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8"/>
    <title>Fleetdock API</title>
    <meta name="viewport" content="width=device-width, initial-scale=1"/>
    <style>body { margin: 0; padding: 0; }</style>
  </head>
  <body>
    <redoc spec-url="/openapi.yaml"></redoc>
    <script src="https://cdn.redocly.com/redoc/latest/bundles/redoc.standalone.js"></script>
  </body>
</html>`

// Page handles GET /docs (renders the spec with Redoc).
func (h *DocsHandler) Page(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(redocPage))
}
