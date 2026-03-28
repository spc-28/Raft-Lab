package api

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// submitRequest is the JSON body accepted by POST /control/submit.
type submitRequest struct {
	Command string `json:"command"`
}

// handleStopNode stops the node identified by the :id path parameter,
// simulating a crash or network partition.
func (s *Server) handleStopNode(c *gin.Context) {
	id, ok := parseNodeID(c)
	if !ok {
		return
	}

	s.controller.StopNode(id)
	c.JSON(http.StatusOK, gin.H{"status": "stopped", "id": id})
}

// handleStartNode restarts a previously stopped node, letting it rejoin the
// cluster and catch up on the replicated log.
func (s *Server) handleStartNode(c *gin.Context) {
	id, ok := parseNodeID(c)
	if !ok {
		return
	}

	s.controller.StartNode(id)
	c.JSON(http.StatusOK, gin.H{"status": "started", "id": id})
}

// handleSubmitCommand forwards a client command to the current leader.
// It returns 503 when no leader is available to accept the write.
func (s *Server) handleSubmitCommand(c *gin.Context) {
	var req submitRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if !s.controller.Submit(req.Command) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no leader available"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "submitted"})
}


func parseNodeID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
		return 0, false
	}
	return id, true
}
