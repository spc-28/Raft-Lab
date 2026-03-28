package api

import (
	"log"
	"raft-lab/raft"
	"github.com/gin-gonic/gin"
)

// Server adapts the simulation's ClusterController to an HTTP/WebSocket API.
type Server struct {
	controller *raft.ClusterController
}

// NewServer wires a Server to the cluster controller it will expose.
func NewServer(controller *raft.ClusterController) *Server {
	return &Server{controller: controller}
}

// Run configures the router and starts listening on addr (e.g. ":8080").
// It blocks until the underlying HTTP server stops.
func (s *Server) Run(addr string) {
	router := gin.Default()
	router.Use(corsMiddleware())

	s.registerRoutes(router)

	log.Printf("🚀 Server listening on http://localhost%s", addr)
	err := router.Run(addr)   // run the server, capture any error
	if err != nil {           // if it failed
		log.Fatalf("server stopped: %v", err)
	}
}

// registerRoutes maps every route to its handler, grouped by responsibility.
func (s *Server) registerRoutes(router *gin.Engine) {
	// Chaos-control endpoints: mutate the running cluster.
	control := router.Group("/control")
	{
		control.POST("/stop/:id", s.handleStopNode)
		control.POST("/start/:id", s.handleStartNode)
		control.POST("/submit", s.handleSubmitCommand)
	}

	// Live cluster-state stream consumed by the frontend visualizer.
	router.GET("/ws", s.handleStateStream)
}
