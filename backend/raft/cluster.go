package raft

// Cluster is a fixed set of nodes with ids 1..n, all sharing one Network.
type Cluster struct {
	nodes []*RaftNode
}

func NewCluster(n int) *Cluster {
	net := Network{}
	for id := 1; id <= n; id++ {
		net[id] = make(chan Message, 100)
	}

	c := &Cluster{}
	for id := 1; id <= n; id++ {
		var peers []int
		for peer := 1; peer <= n; peer++ {
			if peer != id {
				peers = append(peers, peer)
			}
		}
		node := NewRaftNode(id, peers, net)
		c.nodes = append(c.nodes, node)
		go node.Run()
	}
	return c
}

// Node returns the node with the given id, or nil if there is none.
func (c *Cluster) Node(id int) *RaftNode {
	if id < 1 || id > len(c.nodes) {
		return nil
	}
	return c.nodes[id-1]
}

func (c *Cluster) Info() []NodeInfo {
	var infos []NodeInfo
	for _, node := range c.nodes {
		infos = append(infos, node.Info())
	}
	return infos
}

// Submit hands the command to whichever node is currently the live leader.
func (c *Cluster) Submit(cmd string) bool {
	for _, node := range c.nodes {
		if node.Submit(cmd) {
			return true
		}
	}
	return false
}
