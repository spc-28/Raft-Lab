package raft

import (
	"sync"
	"time"
)

type ClusterController struct {
	mu        sync.Mutex
	Nodes     map[int]*RaftNode
	Transport *VirtualTransport
}

func NewClusterController(nodeCount int) *ClusterController {
	transport := NewVirtualTransport()
	nodes := make(map[int]*RaftNode)

	for i := 1; i <= nodeCount; i++ {
		// Peers are every other node (1..N excluding self).
		var peers []int
		for id := 1; id <= nodeCount; id++ {
			if id != i {
				peers = append(peers, id)
			}
		}
		nodes[i] = NewRaftNode(i, peers, transport)
	}

	return &ClusterController{
		Nodes:     nodes,
		Transport: transport,
	}
}

func (cc *ClusterController) Start() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	for _, node := range cc.Nodes {
		node.Start()
	}
}

func (cc *ClusterController) StopNode(id int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if node, ok := cc.Nodes[id]; ok {
		node.Stop()
	}
}

func (cc *ClusterController) StartNode(id int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if node, ok := cc.Nodes[id]; ok {
		node.Resume()
	}
}

// GetState returns a serializable snapshot of the cluster, with nodes ordered
// by ID (map iteration order is random, so we walk 1..N explicitly).
func (cc *ClusterController) GetState() interface{} {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	var states []NodeInfo
	for i := 1; i <= len(cc.Nodes); i++ {
		if node, ok := cc.Nodes[i]; ok {
			states = append(states, node.GetState())
		}
	}

	return map[string]interface{}{
		"nodes":     states,
		"timestamp": time.Now().UnixMilli(),
	}
}

// Submit tries each node in turn; SubmitCommand only succeeds on the leader.
func (cc *ClusterController) Submit(cmd string) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	for _, node := range cc.Nodes {
		if node.SubmitCommand(cmd) {
			return true
		}
	}
	return false
}
