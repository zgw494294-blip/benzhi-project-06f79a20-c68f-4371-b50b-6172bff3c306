package web

import (
	"net/http"
)

func serveAsset(w http.ResponseWriter, name, contentType string) {
	data, err := assets.ReadFile("static/" + name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) WorkbenchPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	serveAsset(w, "index.html", "text/html; charset=utf-8")
}

func (s *Server) ReadingPage(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, "reading.html", "text/html; charset=utf-8")
}
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
