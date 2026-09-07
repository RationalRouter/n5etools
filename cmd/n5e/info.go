// Static, non-database-backed informational pages (About the Creator, Bug
// Reports, FAQ, Technical Overview) — linked from the nav-info tab group in
// layout.html rather than the main content nav-links.
package main

import "net/http"

func (s *server) handleAbout(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "about.html", map[string]any{"Title": s.t(r, "about.title")})
}

func (s *server) handleBugs(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "bugs.html", map[string]any{"Title": s.t(r, "bugs.title")})
}

func (s *server) handleFAQ(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "faq.html", map[string]any{"Title": s.t(r, "faq.title")})
}

func (s *server) handleTechnical(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "technical.html", map[string]any{"Title": s.t(r, "technical.title")})
}
