// Static, non-database-backed informational pages (About the Creator, Bug
// Reports, FAQ, Technical Overview) — linked from the nav-info tab group in
// layout.html rather than the main content nav-links.
package main

import "net/http"

func (s *server) handleAbout(w http.ResponseWriter, r *http.Request) {
	s.render(w, "about.html", map[string]any{"Title": "About the Creator"})
}

func (s *server) handleBugs(w http.ResponseWriter, r *http.Request) {
	s.render(w, "bugs.html", map[string]any{"Title": "Bug Reports"})
}

func (s *server) handleFAQ(w http.ResponseWriter, r *http.Request) {
	s.render(w, "faq.html", map[string]any{"Title": "FAQ"})
}

func (s *server) handleTechnical(w http.ResponseWriter, r *http.Request) {
	s.render(w, "technical.html", map[string]any{"Title": "Technical Overview"})
}
