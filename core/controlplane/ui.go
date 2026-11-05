package controlplane

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed ui/*
var controlPlaneUI embed.FS

func controlPlaneUIHandler() http.Handler {
	sub, err := fs.Sub(controlPlaneUI, "ui")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/ui/", http.FileServer(http.FS(sub)))
}

func handleControlPlaneUI(ui http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		switch {
		case r.URL.Path == "/":
			http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
		case r.URL.Path == "/ui":
			http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
		case strings.HasPrefix(r.URL.Path, "/ui/"):
			ui.ServeHTTP(w, r)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	}
}
