package raft

import (
	"log"
	"math/rand"
	"slices"
	"sync"
	"time"
)

const (
	tickInterval      = 10 * time.Millisecond
	heartbeatInterval = 100 * time.Millisecond
)

// NodeInfo is the part of a node's state the dashboard can see.
type NodeInfo struct {
	ID          int        `json:"id"`
	State       Role       `json:"state"`
	CurrentTerm int        `json:"current_term"`
	VotedFor    int        `json:"voted_for"` // -1 means no vote this term.
	Log         []LogEntry `json:"log"`
	CommitIndex int        `json:"commit_index"` // Highest index known committed; -1 means none.
	LastApplied int        `json:"last_applied"` // Highest index applied to StateMachine; -1 means none.
	IsAlive     bool       `json:"is_alive"`     // false simulates a crashed node.
}

type RaftNode struct {
	mu sync.Mutex // Guards everything below. Held for every state change.
	NodeInfo

	peers        []int
	net          Network
	StateMachine []string // Commands applied in commit order.

	// Leader only: per-peer replication progress.
	nextIndex  map[int]int // Next log index to send to the peer.
	matchIndex map[int]int // Highest log index known to be on the peer.

	votes            int       // Candidate only: votes received this term.
	electionDeadline time.Time // Follower/candidate: start an election after this.
	lastHeartbeat    time.Time // Leader: when we last sent AppendEntries.
}

func NewRaftNode(id int, peers []int, net Network) *RaftNode {
	rn := &RaftNode{
		NodeInfo: NodeInfo{
			ID: id, State: Follower, VotedFor: -1, Log: []LogEntry{},
			CommitIndex: -1, LastApplied: -1, IsAlive: true,
		},
		peers:      peers,
		net:        net,
		nextIndex:  map[int]int{},
		matchIndex: map[int]int{},
	}
	rn.resetElectionTimer()
	return rn
}

// =============================================================================
// Public API (called from HTTP goroutines)
// =============================================================================

// Info returns a copy of the visible state, safe to JSON-encode outside the lock.
func (rn *RaftNode) Info() NodeInfo {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	info := rn.NodeInfo
	info.Log = slices.Clone(rn.Log)
	return info
}

// SetAlive crashes (false) or revives (true) the node. State is kept, so a
// revived node behaves as if it had persisted term, vote and log to disk.
func (rn *RaftNode) SetAlive(alive bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	rn.IsAlive = alive
	rn.resetElectionTimer()
}

// Submit appends a client command if this node is the live leader. The next
// heartbeat replicates it.
func (rn *RaftNode) Submit(cmd string) bool {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	if !rn.IsAlive || rn.State != Leader {
		return false
	}
	rn.Log = append(rn.Log, LogEntry{Term: rn.CurrentTerm, Command: cmd})
	log.Printf("Node %d: accepted %q at index %d", rn.ID, cmd, len(rn.Log)-1)
	return true
}

// =============================================================================
// Event loop: the only goroutine that drives the algorithm
// =============================================================================

func (rn *RaftNode) Run() {
	ticker := time.NewTicker(tickInterval)
	for {
		select {
		case msg := <-rn.net[rn.ID]:
			rn.mu.Lock()
			if rn.IsAlive { // A crashed node loses messages sent to it.
				rn.handleMessage(msg)
			}
			rn.mu.Unlock()
		case <-ticker.C:
			rn.tick()
		}
	}
}

// tick drives everything time-based: heartbeats, election timeouts, applying.
func (rn *RaftNode) tick() {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	if !rn.IsAlive {
		return
	}

	if rn.State == Leader {
		if time.Since(rn.lastHeartbeat) >= heartbeatInterval {
			rn.broadcastAppendEntries()
		}
	} else if time.Now().After(rn.electionDeadline) {
		rn.startElection()
	}

	for rn.LastApplied < rn.CommitIndex {
		rn.LastApplied++
		rn.StateMachine = append(rn.StateMachine, rn.Log[rn.LastApplied].Command)
	}
}

func (rn *RaftNode) handleMessage(msg Message) {
	switch m := msg.Payload.(type) {
	case RequestVoteArgs:
		rn.handleRequestVote(m)
	case RequestVoteReply:
		rn.handleRequestVoteReply(m)
	case AppendEntriesArgs:
		rn.handleAppendEntries(m)
	case AppendEntriesReply:
		rn.handleAppendEntriesReply(msg.From, m)
	}
}

// =============================================================================
// Leader election
// =============================================================================

func (rn *RaftNode) startElection() {
	rn.State = Candidate
	rn.CurrentTerm++
	rn.VotedFor = rn.ID
	rn.votes = 1
	rn.resetElectionTimer() // If this election splits, retry after a new random timeout.
	log.Printf("Node %d: starting election for term %d", rn.ID, rn.CurrentTerm)

	lastIndex, lastTerm := rn.lastLog()
	for _, peer := range rn.peers {
		rn.send(peer, RequestVoteArgs{rn.CurrentTerm, rn.ID, lastIndex, lastTerm})
	}
}

func (rn *RaftNode) handleRequestVote(args RequestVoteArgs) {
	rn.observeTerm(args.Term)

	// Grant only if the candidate's log is at least as up to date as ours (§5.4.1).
	lastIndex, lastTerm := rn.lastLog()
	logOK := args.LastLogTerm > lastTerm ||
		(args.LastLogTerm == lastTerm && args.LastLogIndex >= lastIndex)

	granted := args.Term == rn.CurrentTerm && logOK &&
		(rn.VotedFor == -1 || rn.VotedFor == args.CandidateID)
	if granted {
		rn.VotedFor = args.CandidateID
		rn.resetElectionTimer()
	}
	rn.send(args.CandidateID, RequestVoteReply{rn.CurrentTerm, granted})
}

func (rn *RaftNode) handleRequestVoteReply(reply RequestVoteReply) {
	rn.observeTerm(reply.Term)
	if rn.State != Candidate || reply.Term != rn.CurrentTerm || !reply.VoteGranted {
		return
	}
	rn.votes++
	if rn.votes >= rn.majority() {
		rn.becomeLeader()
	}
}

func (rn *RaftNode) becomeLeader() {
	log.Printf("Node %d: won election for term %d", rn.ID, rn.CurrentTerm)
	rn.State = Leader
	for _, peer := range rn.peers {
		rn.nextIndex[peer] = len(rn.Log)
		rn.matchIndex[peer] = -1
	}
	rn.broadcastAppendEntries() // Announce leadership immediately.
}

// =============================================================================
// Log replication
// =============================================================================

// broadcastAppendEntries sends each peer the entries it is missing (none = heartbeat).
func (rn *RaftNode) broadcastAppendEntries() {
	rn.lastHeartbeat = time.Now()
	for _, peer := range rn.peers {
		prevIndex := rn.nextIndex[peer] - 1
		prevTerm := 0
		if prevIndex >= 0 {
			prevTerm = rn.Log[prevIndex].Term
		}
		entries := slices.Clone(rn.Log[rn.nextIndex[peer]:])
		rn.send(peer, AppendEntriesArgs{rn.CurrentTerm, rn.ID, prevIndex, prevTerm, entries, rn.CommitIndex})
	}
}

func (rn *RaftNode) handleAppendEntries(args AppendEntriesArgs) {
	rn.observeTerm(args.Term)
	if args.Term < rn.CurrentTerm {
		rn.send(args.LeaderID, AppendEntriesReply{Term: rn.CurrentTerm})
		return
	}
	// A valid leader exists for our term: candidates give up, everyone resets their timer.
	rn.State = Follower
	rn.resetElectionTimer()

	// Consistency check: we must already hold the entry right before the new ones.
	// PrevLogIndex == -1 means "start of the log" and always matches.
	if args.PrevLogIndex >= len(rn.Log) ||
		(args.PrevLogIndex >= 0 && rn.Log[args.PrevLogIndex].Term != args.PrevLogTerm) {
		rn.send(args.LeaderID, AppendEntriesReply{Term: rn.CurrentTerm})
		return
	}

	// Append new entries. Truncate only on a real conflict (same index, different
	// term), so a delayed, shorter RPC can't erase entries we already have.
	for i, entry := range args.Entries {
		idx := args.PrevLogIndex + 1 + i
		if idx < len(rn.Log) && rn.Log[idx].Term != entry.Term {
			rn.Log = rn.Log[:idx]
		}
		if idx >= len(rn.Log) {
			rn.Log = append(rn.Log, entry)
		}
	}

	lastNew := args.PrevLogIndex + len(args.Entries)
	if args.LeaderCommit > rn.CommitIndex {
		rn.CommitIndex = min(args.LeaderCommit, lastNew)
	}
	rn.send(args.LeaderID, AppendEntriesReply{Term: rn.CurrentTerm, Success: true, MatchIndex: lastNew})
}

func (rn *RaftNode) handleAppendEntriesReply(from int, reply AppendEntriesReply) {
	rn.observeTerm(reply.Term)
	if rn.State != Leader || reply.Term != rn.CurrentTerm {
		return
	}
	if !reply.Success {
		// Logs diverge: step back one entry and retry on the next heartbeat.
		rn.nextIndex[from] = max(rn.nextIndex[from]-1, 0)
		return
	}
	rn.matchIndex[from] = max(rn.matchIndex[from], reply.MatchIndex) // Ignore stale replies.
	rn.nextIndex[from] = rn.matchIndex[from] + 1

	// Commit the highest entry stored on a majority, but only if it is from our
	// own term; older entries then commit indirectly (§5.4.2).
	for n := len(rn.Log) - 1; n > rn.CommitIndex && rn.Log[n].Term == rn.CurrentTerm; n-- {
		count := 1 // Ourselves.
		for _, peer := range rn.peers {
			if rn.matchIndex[peer] >= n {
				count++
			}
		}
		if count >= rn.majority() {
			rn.CommitIndex = n
			return
		}
	}
}

// =============================================================================
// Helpers (caller holds rn.mu)
// =============================================================================

// observeTerm applies Raft's universal rule: seeing a newer term makes us a
// follower in that term with no vote cast yet.
func (rn *RaftNode) observeTerm(term int) {
	if term > rn.CurrentTerm {
		rn.CurrentTerm = term
		rn.VotedFor = -1
		rn.State = Follower
	}
}

func (rn *RaftNode) resetElectionTimer() {
	rn.electionDeadline = time.Now().Add(time.Duration(300+rand.Intn(300)) * time.Millisecond)
}

func (rn *RaftNode) lastLog() (index, term int) {
	index = len(rn.Log) - 1
	if index >= 0 {
		term = rn.Log[index].Term
	}
	return index, term
}

func (rn *RaftNode) majority() int { return (len(rn.peers)+1)/2 + 1 }

func (rn *RaftNode) send(to int, payload any) {
	rn.net.Send(Message{From: rn.ID, To: to, Payload: payload})
}
