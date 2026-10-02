package raft

type Role string

const (
	Follower  Role = "FOLLOWER"
	Candidate Role = "CANDIDATE"
	Leader    Role = "LEADER"
)

type LogEntry struct {
	Term    int    `json:"term"`
	Command string `json:"command"`
}

// The four RPC payloads from Figure 2 of the Raft paper.

type RequestVoteArgs struct {
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type AppendEntriesArgs struct {
	Term         int
	LeaderID     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term       int
	Success    bool
	MatchIndex int // Last log index the follower now shares with the leader.
}

// Message is one RPC travelling between nodes; Payload is one of the four types above.
type Message struct {
	From, To int
	Payload  any
}

// Network is the simulated network: one buffered inbox channel per node id.
type Network map[int]chan Message

// Send never blocks: if the receiver's inbox is full the message is dropped,
// just like a lossy real network. Raft retries, so drops are harmless.
func (n Network) Send(msg Message) {
	select {
	case n[msg.To] <- msg:
	default:
	}
}
