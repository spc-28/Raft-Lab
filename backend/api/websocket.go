package api

import (
	"log"
	"net/http"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// stateBroadcastInterval controls how often a full cluster-state snapshot is
// pushed to each connected client. It matches the controller's tick rate.
const stateBroadcastInterval = 100 * time.Millisecond

// wsUpgrader upgrades incoming GET /ws requests to a WebSocket connection.
// CheckOrigin is permissive because this is a local simulation tool.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleStateStream upgrades the connection and streams cluster-state snapshots
// to the client on a fixed interval until the socket closes.
func (s *Server) handleStateStream(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("ws upgrade error:", err)
		return
	}
	defer conn.Close()

	// Drain inbound frames in the background so control/ping/close messages are
	// processed and don't pile up on the connection.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(stateBroadcastInterval)
	defer ticker.Stop()

	// Push a snapshot on every tick. When the client disconnects, the next write
	// fails and we exit.
	for range ticker.C {
		if err := conn.WriteJSON(s.controller.GetState()); err != nil {
			log.Println("ws write error:", err)
			return
		}
	}
}
