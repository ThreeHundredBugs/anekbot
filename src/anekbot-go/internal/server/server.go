package server

import (
	"net/http"
)

func New(addr, webhookPath string, webhookHandler http.HandlerFunc) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(webhookPath, webhookHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}
