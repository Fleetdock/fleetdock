package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	authzapp "github.com/Fleetdock/fleetdock/backend/internal/app/authz"
	catalogapp "github.com/Fleetdock/fleetdock/backend/internal/app/catalog"
	"github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	catalogdom "github.com/Fleetdock/fleetdock/backend/internal/domain/catalog"
)

// SearchHandler answers fleet-wide schema searches.
type SearchHandler struct {
	catalog  *catalogapp.Service
	access   dataAccessLookup
	resolver *authzapp.Resolver
}

// NewSearchHandler builds the handler. access tells how each database's
// interactive features connect, for the same gate the data browser applies.
func NewSearchHandler(catalog *catalogapp.Service, access dataAccessLookup, resolver *authzapp.Resolver) *SearchHandler {
	return &SearchHandler{catalog: catalog, access: access, resolver: resolver}
}

// Schema handles GET /v1/search/schema?q=&limit=. A hit is returned only if
// the caller could open its database in the data browser: database:read on
// it plus, for system databases or admin-login instances, instance:write.
// Table names are data too.
func (h *SearchHandler) Schema(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > catalogdom.MaxSearchLimit {
		limit = 20
	}
	if len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []catalogdom.Hit{}})
		return
	}
	// Over-fetch: some hits are dropped by the per-database check below.
	hits, err := h.catalog.Search(r.Context(), q, 4*catalogdom.MaxSearchLimit)
	if err != nil {
		writeError(w, err)
		return
	}
	allowed := map[uuid.UUID]bool{}
	out := make([]catalogdom.Hit, 0, limit)
	for _, hit := range hits {
		ok, seen := allowed[hit.DatabaseID]
		if !seen {
			ok = authorizeResource(r.Context(), h.resolver, "database:read", authz.ResourceDatabase, hit.DatabaseID) == nil &&
				authorizeDataAccess(r.Context(), h.resolver, h.access, hit.DatabaseID) == nil
			allowed[hit.DatabaseID] = ok
		}
		if !ok {
			continue
		}
		out = append(out, hit)
		if len(out) == limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
