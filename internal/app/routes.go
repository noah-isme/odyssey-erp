package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// RouteEntry describes a single route registered on the application router.
type RouteEntry struct {
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
}

// WalkRoutes returns every route registered on the router, sorted and
// deduplicated. It lets callers derive coverage from the real routing table
// instead of a hand-maintained list that silently drifts.
func WalkRoutes(handler http.Handler) ([]RouteEntry, error) {
	routes, ok := handler.(chi.Routes)
	if !ok {
		return nil, fmt.Errorf("handler %T does not expose chi.Routes", handler)
	}

	seen := make(map[RouteEntry]struct{})
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// Subrouters yield patterns with duplicated or trailing slashes.
		route = strings.ReplaceAll(route, "//", "/")
		if len(route) > 1 {
			route = strings.TrimSuffix(route, "/")
		}
		seen[RouteEntry{Method: strings.ToUpper(method), Pattern: route}] = struct{}{}
		return nil
	}
	if err := chi.Walk(routes, walk); err != nil {
		return nil, fmt.Errorf("walk routes: %w", err)
	}

	entries := make([]RouteEntry, 0, len(seen))
	for entry := range seen {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Pattern != entries[j].Pattern {
			return entries[i].Pattern < entries[j].Pattern
		}
		return entries[i].Method < entries[j].Method
	})
	return entries, nil
}

// WriteRoutes emits the router's routes as JSON.
func WriteRoutes(handler http.Handler, w io.Writer) error {
	entries, err := WalkRoutes(handler)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}
