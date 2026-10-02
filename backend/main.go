package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"raft-lab/raft"
)

func main() {
	cluster := raft.NewCluster(5)

	// Crash or revive a node: POST /control/stop/3, POST /control/start/3
	setAlive := func(alive bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.Atoi(r.PathValue("id")) // Non-numeric id becomes 0, which Node rejects.
			node := cluster.Node(id)
			if node == nil {
				http.Error(w, "unknown node id", http.StatusBadRequest)
				return
			}
			node.SetAlive(alive)
		}
	}
	http.HandleFunc("POST /control/stop/{id}", setAlive(false))
	http.HandleFunc("POST /control/start/{id}", setAlive(true))

	// Client write: POST /control/submit {"command": "x"}
	http.HandleFunc("POST /control/submit", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Command string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !cluster.Submit(req.Command) {
			http.Error(w, "no leader available", http.StatusServiceUnavailable)
		}
	})

	// Live dashboard feed: push the whole cluster state every 100ms.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	http.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		go func() { // Read (and discard) client frames so close/ping are handled.
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		for range time.Tick(100 * time.Millisecond) {
			if conn.WriteJSON(map[string]any{"nodes": cluster.Info()}) != nil {
				return // Client went away.
			}
		}
	})

	log.Println("Backend listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", allowCORS(http.DefaultServeMux)))
}

// allowCORS lets the Vite dev server (another origin) call this API.
func allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions { // Preflight.
			return
		}
		next.ServeHTTP(w, r)
	})
}
