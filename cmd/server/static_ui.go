package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func registerStaticUIRoutes(mux *http.ServeMux, log *slog.Logger) {
	const distDir = "web/dist"
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		log.Warn("ui dist assets not found; skipping static ui routes", "path", indexPath)
		return
	}
	fs := http.FileServer(http.Dir(distDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/hooks/") {
			http.NotFound(w, r)
			return
		}
		cleanPath := filepath.Clean(r.URL.Path)
		trimmed := strings.TrimPrefix(cleanPath, "/")
		target := filepath.Join(distDir, trimmed)
		if cleanPath == "/" {
			target = indexPath
		}
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, indexPath)
	}))
	log.Info("serving ui static assets", "dist", distDir)
}
