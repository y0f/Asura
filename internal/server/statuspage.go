package server

import (
	"context"
	"net/http"
	"strings"
)

func (s *Server) refreshStatusSlugs() {
	pages, err := s.store.ListStatusPages(context.Background())
	if err != nil {
		s.logger.Error("refresh status slugs", "error", err)
		return
	}
	slugs := make(map[string]int64, len(pages))
	for _, p := range pages {
		if p.Enabled {
			slugs[p.Slug] = p.ID
		}
	}
	s.statusSlugsMu.Lock()
	s.statusSlugs = slugs
	s.statusSlugsMu.Unlock()
}

func (s *Server) getStatusPageIDBySlug(slug string) (int64, bool) {
	s.statusSlugsMu.RLock()
	defer s.statusSlugsMu.RUnlock()
	id, ok := s.statusSlugs[slug]
	return id, ok
}

func (s *Server) statusPageRouter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && s.web != nil {
			path := r.URL.Path
			prefix := s.cfg.Server.BasePath + "/"
			if strings.HasPrefix(path, prefix) {
				slug := strings.TrimPrefix(path, prefix)
				slug = strings.TrimSuffix(slug, "/")
				if slug != "" && !strings.Contains(slug, "/") {
					if pageID, ok := s.getStatusPageIDBySlug(slug); ok {
						s.web.StatusPageByID(w, r, pageID)
						return
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
