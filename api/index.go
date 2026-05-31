package handler

import (
	"log"
	"net/http"
	"os"
	"sync"

	"transbridge/internal/app"
)

var (
	application *app.App
	initErr     error
	initOnce    sync.Once
)

// Handler 是 Vercel Go Runtime 的入口。
func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(func() {
		configFile := os.Getenv("TRANSBRIDGE_CONFIG")
		if configFile == "" {
			configFile = "config.yml"
		}

		application, initErr = app.New(configFile, app.Options{
			Serverless: true,
		})
		if initErr != nil {
			log.Printf("initialize vercel application failed: %v", initErr)
		}
	})

	if initErr != nil {
		http.Error(w, "application initialization failed", http.StatusInternalServerError)
		return
	}

	application.Handler.ServeHTTP(w, normalizeRequestPath(r))
}

func normalizeRequestPath(r *http.Request) *http.Request {
	path := r.URL.Query().Get("__path")
	if path == "" {
		return r
	}

	cloned := r.Clone(r.Context())
	urlCopy := *r.URL
	query := urlCopy.Query()
	query.Del("__path")
	urlCopy.Path = path
	urlCopy.RawQuery = query.Encode()
	cloned.URL = &urlCopy
	return cloned
}
