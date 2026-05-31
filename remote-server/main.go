package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		frontendDir = "./frontend"
	}

	store := NewStore()
	h := NewHandler(store)

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/sessions", h.handleSessions)
	mux.HandleFunc("/api/sessions/", h.handleSessionByID)
	mux.HandleFunc("/api/events", h.handleEvents)
	mux.HandleFunc("/api/messages", h.handleMessages)
	mux.HandleFunc("/api/transcripts", h.handleTranscripts)
	mux.HandleFunc("/api/tools", h.handleTools)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Serve frontend
	mux.Handle("/", http.FileServer(http.Dir(frontendDir)))

	log.Printf("Remote session server starting on :%s (frontend: %s)", port, frontendDir)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
