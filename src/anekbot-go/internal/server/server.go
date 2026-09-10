// Package server wires the HTTP server used in webhook mode: Telegram's
// webhook endpoint plus a health check, with graceful shutdown.
package server

import (
	"net/http"
)

// New builds an *http.Server serving the bot's webhook handler at
// webhookPath, plus a GET /healthz endpoint for process supervisors.
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
