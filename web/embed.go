// Package web embeds the built React UI (web/dist) into the binary.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the UI. Normally that is the embedded build. When devServer
// is set (only `make run` does this), requests are proxied to the Vite dev
// server instead, so development uses the same single URL as production while
// keeping Vite's hot reload; its websocket upgrades pass through the proxy.
func Handler(devServer string) (http.Handler, error) {
	if devServer == "" {
		return embedded(), nil
	}
	target, err := url.Parse(devServer)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("UI_DEV_SERVER must be a URL like http://127.0.0.1:5173, got %q", devServer)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(target) },
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			// Vite is usually just still starting; retry shortly.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `<!doctype html><meta http-equiv="refresh" content="1"><title>Starting…</title>`+
				`<p style="font:14px system-ui;padding:2rem;color:#71717a">Waiting for the UI dev server to start…</p>`)
		},
	}
	return proxy, nil
}

// embedded serves the single-page app from the binary: real files are served
// as-is and every other path falls back to index.html so client-side routes
// work on reload.
func embedded() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" {
			if info, err := fs.Stat(root, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					// Vite fingerprints everything under assets/.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			http.Error(w, "UI not built. Run `make web` (or use the Docker image).", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
