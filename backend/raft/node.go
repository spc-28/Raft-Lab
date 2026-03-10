package raft

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// =============================================================================
// Type
// =============================================================================

type RaftNode struct {
	mu sync.Mutex // Guards every mutable field below; held during all state changes.


	ID    int   // This node's unique id.
	Peers []int // Ids of all other nodes (excludes self).


	CurrentTerm int
	VotedFor    int        // Candidate voted for in CurrentTerm; -1 means "none yet".
	Log         []LogEntry // Replicated command log; index 0 is the first entry.


	CommitIndex int // Highest log index known to be committed; -1 means none.
	LastApplied int // Highest log index applied to StateMachine; -1 means none.


	StateMachine map[int]string // Result of applying committed entries, keyed by log index.

	// only useful to leader
	NextIndex  map[int]int // Per peer: next log index to send them.
	MatchIndex map[int]int // Per peer: highest log index known replicated on them.


	State         Role      // Current role: Follower, Candidate, or Leader.
	LastHeartbeat time.Time // When we last heard from a valid leader / granted a vote; drives election timeout.


	IsAlive bool // False simulates a crashed node: loops idle until Resume.


	Transport *VirtualTransport // In-memory network used to send messages to peers.
	Inbox     chan Message      // Incoming messages addressed to this node.
}

// =============================================================================
// Liveness & state snapshots
// =============================================================================

func (rn *RaftNode) Stop() {
	rn.mu.Lock()
	rn.IsAlive = false
	rn.mu.Unlock()
}

func (rn *RaftNode) Resume() {
	rn.mu.Lock()
	rn.IsAlive = true
	rn.LastHeartbeat = time.Now()
	rn.mu.Unlock()
}

// NodeInfo is a snapshot of the node state
type NodeInfo struct {
	ID          int        `json:"id"`           // Which node this snapshot describes.
	State       Role       `json:"state"`        // Role at snapshot time (Follower/Candidate/Leader).
	CurrentTerm int        `json:"current_term"` // Term at snapshot time.
	VotedFor    int        `json:"voted_for"`    // Vote in the current term; -1 means none.
	CommitIndex int        `json:"commit_index"` // Highest committed log index.
	LastApplied int        `json:"last_applied"` // Highest applied log index.
	IsAlive     bool       `json:"is_alive"`     // Whether the node is running (not simulated-crashed).
	Log         []LogEntry `json:"log"`          // Defensive copy of the log for safe marshaling.
}

// Thread-safe state getter for visualization
func (rn *RaftNode) GetState() NodeInfo {
	rn.mu.Lock()
	defer rn.mu.Unlock() // runs once the function returns at any point

	// Copy log to avoid race conditions during JSON marshaling outside lock
	logCopy := make([]LogEntry, len(rn.Log))
	copy(logCopy, rn.Log)

	return NodeInfo{
		ID:          rn.ID,
		State:       rn.State,
		CurrentTerm: rn.CurrentTerm,
		VotedFor:    rn.VotedFor,
		CommitIndex: rn.CommitIndex,
		LastApplied: rn.LastApplied,
		IsAlive:     rn.IsAlive,
		Log:         logCopy,
	}
}

// =============================================================================
// State machine application
// =============================================================================

// background worker that constantly checks: "Are there committed entries I haven't run yet? If so, run them.
func (rn *RaftNode) applyLoop() {
	for {
		rn.mu.Lock()
		if rn.IsAlive {
			for rn.LastApplied < rn.CommitIndex {
				rn.LastApplied++
				entry := rn.Log[rn.LastApplied]
				rn.StateMachine[rn.LastApplied] = entry.Command
			}
		}
		rn.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
}

// =============================================================================
// Outbound messages (client commands & broadcasts)
// =============================================================================

func (rn *RaftNode) broadcastRequestVote() {
	rn.mu.Lock()
	lastLogIndex := len(rn.Log) - 1
	lastLogTerm := 0
	if lastLogIndex >= 0 {
		lastLogTerm = rn.Log[lastLogIndex].Term
	}

	args := RequestVoteArgs{
		Term:         rn.CurrentTerm,
		CandidateID:  rn.ID,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}
	peers := rn.Peers
	rn.mu.Unlock()
	// the next step of sending can be slow, so we released the lock above
	for _, peer := range peers {
		rn.Transport.Send(Message{
			From:    rn.ID,
			To:      peer,
			Type:    "RequestVote",
			Payload: args,
		})
	}
}

func (rn *RaftNode) broadcastAppendEntries() {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	peers := rn.Peers
	currentTerm := rn.CurrentTerm
	leaderID := rn.ID
	leaderCommit := rn.CommitIndex

	for _, peer := range peers {
		// Calculate PrevLogIndex and Entries for this peer
		prevLogIndex := rn.NextIndex[peer] - 1
		prevLogTerm := 0
		if prevLogIndex >= 0 && prevLogIndex < len(rn.Log) {
			prevLogTerm = rn.Log[prevLogIndex].Term
		}

		var entries []LogEntry
		if rn.NextIndex[peer] < len(rn.Log) {
			// Need to send entries
			entries = rn.Log[rn.NextIndex[peer]:]
		}

		args := AppendEntriesArgs{
			Term:         currentTerm,
			LeaderID:     leaderID,
			PrevLogIndex: prevLogIndex,
			PrevLogTerm:  prevLogTerm,
			Entries:      entries,
			LeaderCommit: leaderCommit,
		}

		rn.Transport.Send(Message{
			From:    leaderID,
			To:      peer,
			Type:    "AppendEntries",
			Payload: args,
		})
	}
}

// SubmitCommand allows external clients to propose a command
func (rn *RaftNode) SubmitCommand(cmd string) bool {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.State != Leader {
		return false
	}

	newEntry := LogEntry{
		Term:    rn.CurrentTerm,
		Command: cmd,
	}
	rn.Log = append(rn.Log, newEntry)
	fmt.Printf("Node %d: Received command '%s' at index %d\n", rn.ID, cmd, len(rn.Log)-1)

	// The heartbeat ticker replicates this entry on its next tick (≤100ms).
	return true
}

// =============================================================================
// Inbound message handlers
// =============================================================================

func (rn *RaftNode) handleMessage(msg Message) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	switch msg.Type {
	case "RequestVote":
		rn.handleRequestVote(msg.From, decode[RequestVoteArgs](msg.Payload))

	case "AppendEntries":
		rn.handleAppendEntries(msg.From, decode[AppendEntriesArgs](msg.Payload))

	case "RequestVoteReply":
		reply := decode[RequestVoteReply](msg.Payload)
		if reply.Term > rn.CurrentTerm {
			rn.stepDown(reply.Term)
		}

	case "AppendEntriesReply":
		reply := decode[AppendEntriesReply](msg.Payload)
		if reply.Term > rn.CurrentTerm {
			rn.stepDown(reply.Term)
			return
		}
		if rn.State == Leader {
			rn.handleAppendEntriesReply(msg.From, reply)
		}
	}
}

func (rn *RaftNode) handleRequestVote(from int, args RequestVoteArgs) {
	reply := RequestVoteReply{
		Term:        rn.CurrentTerm,
		VoteGranted: false,
	}

	// 1. Reply false if term < currentTerm
	if args.Term < rn.CurrentTerm {
		rn.Transport.Send(Message{From: rn.ID, To: from, Type: "RequestVoteReply", Payload: reply})
		return
	}

	// Newer term seen: revert to follower before considering the vote.
	if args.Term > rn.CurrentTerm {
		rn.stepDown(args.Term)
	}

	reply.Term = rn.CurrentTerm

	// Check Log Up-to-Date
	lastLogIndex := len(rn.Log) - 1
	lastLogTerm := 0
	if lastLogIndex >= 0 {
		lastLogTerm = rn.Log[lastLogIndex].Term
	}

	isLogOk := (args.LastLogTerm > lastLogTerm) || (args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)

	// 2. If votedFor is null or candidateId, and candidate’s log is at least as up-to-date as receiver’s log, grant vote
	if (rn.VotedFor == -1 || rn.VotedFor == args.CandidateID) && isLogOk {
		rn.VotedFor = args.CandidateID
		reply.VoteGranted = true
		rn.LastHeartbeat = time.Now() // Granting vote resets election timeout!
		fmt.Printf("Node %d: Voted for Node %d in term %d\n", rn.ID, args.CandidateID, rn.CurrentTerm)
	}

	rn.Transport.Send(Message{From: rn.ID, To: from, Type: "RequestVoteReply", Payload: reply})
}

func (rn *RaftNode) handleAppendEntries(from int, args AppendEntriesArgs) {
	reply := AppendEntriesReply{
		Term:    rn.CurrentTerm,
		Success: false,
	}

	if args.Term < rn.CurrentTerm {
		rn.Transport.Send(Message{From: rn.ID, To: from, Type: "AppendEntriesReply", Payload: reply})
		return
	}

	if args.Term > rn.CurrentTerm || rn.State != Follower {
		rn.stepDown(args.Term)
	}

	rn.LastHeartbeat = time.Now()
	reply.Term = rn.CurrentTerm

	// Log consistency check. Indices are 0-based; the leader sends
	// PrevLogIndex = NextIndex-1, so PrevLogIndex == -1 means "before the start"
	// of an (empty) log and always matches.
	if args.PrevLogIndex >= len(rn.Log) {
		// We don't have this entry yet.
		reply.Success = false
		rn.Transport.Send(Message{From: rn.ID, To: from, Type: "AppendEntriesReply", Payload: reply})
		return
	}

	if args.PrevLogIndex >= 0 {
		if rn.Log[args.PrevLogIndex].Term != args.PrevLogTerm {
			// Term mismatch at PrevLogIndex: reply false only. The leader will
			// back off NextIndex and retry. Do NOT truncate here — truncation
			// only happens when an actual new entry conflicts (below).
			reply.Success = false
			rn.Transport.Send(Message{From: rn.ID, To: from, Type: "AppendEntriesReply", Payload: reply})
			return
		}
	}

	// Append new entries
	// We need to merge args.Entries into rn.Log starting at args.PrevLogIndex + 1
	insertIndex := args.PrevLogIndex + 1
	for i, entry := range args.Entries {
		idx := insertIndex + i
		if idx < len(rn.Log) {
			if rn.Log[idx].Term != entry.Term {
				// Conflict, truncate and replace
				rn.Log = rn.Log[:idx]
				rn.Log = append(rn.Log, entry)
			}
		} else {
			rn.Log = append(rn.Log, entry)
		}
	}

	if args.LeaderCommit > rn.CommitIndex {
		lastNewIndex := insertIndex + len(args.Entries) - 1
		if args.LeaderCommit < lastNewIndex {
			rn.CommitIndex = args.LeaderCommit
		} else {
			rn.CommitIndex = lastNewIndex
		}
	}

	reply.Success = true
	// Report the exact index this RPC brought us up to, so the leader advances
	// MatchIndex by what was actually confirmed rather than its own log end.
	reply.MatchIndex = args.PrevLogIndex + len(args.Entries)
	rn.Transport.Send(Message{From: rn.ID, To: from, Type: "AppendEntriesReply", Payload: reply})
}

// handleAppendEntriesReply processes a follower's response while we are leader:
// it advances that peer's replication progress and, once a current-term entry
// is stored on a majority, advances CommitIndex. Caller holds rn.mu.
func (rn *RaftNode) handleAppendEntriesReply(from int, reply AppendEntriesReply) {
	if !reply.Success {
		// Follower rejected the consistency check: back NextIndex off one and
		// retry on the next heartbeat.
		if rn.NextIndex[from] > 0 {
			rn.NextIndex[from]--
		}
		return
	}

	// Advance MatchIndex to exactly what this reply confirmed. The max guard keeps
	// it monotonic so a stale/reordered reply can't pull it backward; NextIndex
	// follows from MatchIndex.
	if reply.MatchIndex > rn.MatchIndex[from] {
		rn.MatchIndex[from] = reply.MatchIndex
	}
	rn.NextIndex[from] = rn.MatchIndex[from] + 1

	// Advance CommitIndex to the highest N > CommitIndex stored on a majority
	// whose Log[N].Term == CurrentTerm. The current-term check is mandatory
	// (§5.4.2): never commit a prior-term entry purely by replica count.
	majority := (len(rn.Peers)+1)/2 + 1
	for N := len(rn.Log) - 1; N > rn.CommitIndex; N-- {
		if rn.Log[N].Term != rn.CurrentTerm {
			continue
		}
		count := 1 // leader replicates to itself
		for _, peer := range rn.Peers {
			if rn.MatchIndex[peer] >= N {
				count++
			}
		}
		if count >= majority {
			rn.CommitIndex = N
			break
		}
	}
}

// stepDown reverts to follower after observing a newer term. (resolves multi leader co-exists)
func (rn *RaftNode) stepDown(term int) {
	rn.CurrentTerm = term
	rn.State = Follower
	rn.VotedFor = -1
}

// decode returns the message payload as type T. Messages travel in-process, so
// the payload already holds a concrete T; the JSON round-trip is a fallback for
// the case a payload ever arrives in map form.
func decode[T any](payload interface{}) T {
	if v, ok := payload.(T); ok {
		return v
	}
	var out T
	b, _ := json.Marshal(payload)
	_ = json.Unmarshal(b, &out)
	return out
}

// =============================================================================
// Role state-machine loops
// =============================================================================

// run is the master loop: it keeps the node in the routine matching its current
// role. Each runX returns the moment the role changes, then we re-dispatch.
func (rn *RaftNode) run() {
	for {
		// Check liveness
		rn.mu.Lock()
		if !rn.IsAlive {
			rn.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		state := rn.State
		rn.mu.Unlock()

		switch state {
		case Follower:
			rn.runFollower()
		case Candidate:
			rn.runCandidate()
		case Leader:
			rn.runLeader()
		}
	}
}

func (rn *RaftNode) runFollower() {
	rn.mu.Lock()
	timeoutDuration := time.Duration(300+rand.Intn(300)) * time.Millisecond
	rn.mu.Unlock()

	timer := time.NewTimer(timeoutDuration)
	defer timer.Stop()

	for {
		rn.mu.Lock()
		if !rn.IsAlive { // Check liveness
			rn.mu.Unlock()
			return
		}
		if rn.State != Follower {
			rn.mu.Unlock()
			return
		}
		rn.mu.Unlock()

		select {
		case <-timer.C:
			rn.mu.Lock()
			// Check if we really timed out or if we just processed a message that reset it
			if time.Since(rn.LastHeartbeat) >= timeoutDuration {
				fmt.Printf("Node %d: Election timeout. Becoming Candidate.\n", rn.ID)
				rn.State = Candidate
				rn.mu.Unlock()
				return
			}
			// Reset timer
			timer.Reset(timeoutDuration) // simplified
			rn.mu.Unlock()

		case msg := <-rn.Inbox:
			rn.handleMessage(msg)
			// Reset timer on valid leader message
			// The handleMessage updates LastHeartbeat if valid
			rn.mu.Lock()
			// Simple reset
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeoutDuration)
			rn.mu.Unlock()
		}
	}
}

func (rn *RaftNode) runCandidate() {
	rn.mu.Lock()
	rn.CurrentTerm++
	rn.VotedFor = rn.ID
	rn.State = Candidate
	term := rn.CurrentTerm
	rn.LastHeartbeat = time.Now() // prevent immediate timeout
	rn.mu.Unlock()

	fmt.Printf("Node %d: Starting election for term %d\n", rn.ID, term)

	// Send RequestVote to all peers
	votesReceived := 1 // Vote for self
	votesNeeded := (len(rn.Peers)+1)/2 + 1
	// Note: Peers list doesn't include self. So total nodes = len(Peers) + 1.

	go rn.broadcastRequestVote()

	timeoutDuration := time.Duration(300+rand.Intn(300)) * time.Millisecond
	timer := time.NewTimer(timeoutDuration)
	defer timer.Stop()

	for {
		rn.mu.Lock()
		if !rn.IsAlive { // Check liveness
			rn.mu.Unlock()
			return
		}
		if rn.State != Candidate {
			rn.mu.Unlock()
			return
		}
		if votesReceived >= votesNeeded {
			fmt.Printf("Node %d: Won election for term %d\n", rn.ID, rn.CurrentTerm)
			rn.State = Leader

			// Re-initialize leader state
			for _, peer := range rn.Peers {
				rn.NextIndex[peer] = len(rn.Log) // or +1 based on indexing
				rn.MatchIndex[peer] = -1         // Changed from 0 to -1
			}

			rn.mu.Unlock()
			return
		}
		rn.mu.Unlock()

		select {
		case <-timer.C:
			// Election timeout -> Start new election (loop will restart runCandidate)
			return
		case msg := <-rn.Inbox:
			rn.handleMessage(msg)

			// Tally votes. Term step-down (reply.Term > CurrentTerm) is already
			// handled inside handleMessage, so here we only count grants — under
			// the lock, since we read CurrentTerm/State.
			if msg.Type == "RequestVoteReply" {
				if reply, ok := msg.Payload.(RequestVoteReply); ok {
					rn.mu.Lock()
					if rn.State == Candidate && reply.Term == rn.CurrentTerm && reply.VoteGranted {
						votesReceived++
					}
					rn.mu.Unlock()
				}
			}
		}
	}
}

func (rn *RaftNode) runLeader() {
	rn.mu.Lock()
	// Re-assert leader state just in case
	rn.State = Leader
	rn.mu.Unlock()

	ticker := time.NewTicker(100 * time.Millisecond) // Heartbeat interval
	defer ticker.Stop()

	// Immediately send first heartbeat
	rn.broadcastAppendEntries() // Changed from broadcastHeartbeat

	for {
		rn.mu.Lock()
		if !rn.IsAlive { // Check liveness
			rn.mu.Unlock()
			return
		}
		if rn.State != Leader {
			rn.mu.Unlock()
			return
		}
		rn.mu.Unlock()

		select {
		case <-ticker.C:
			rn.broadcastAppendEntries() // Changed from broadcastHeartbeat
		case msg := <-rn.Inbox:
			rn.handleMessage(msg)
		}
	}
}

// =============================================================================
// Construction & startup
// =============================================================================

func NewRaftNode(id int, peers []int, transport *VirtualTransport) *RaftNode {
	inbox := transport.Register(id)

	rn := &RaftNode{
		ID:            id,
		Peers:         peers,
		State:         Follower,
		VotedFor:      -1, // -1 means null
		Log:           make([]LogEntry, 0),
		StateMachine:  make(map[int]string),
		Transport:     transport,
		IsAlive:       true,
		CommitIndex:   -1,
		LastApplied:   -1,
		NextIndex:     make(map[int]int),
		MatchIndex:    make(map[int]int),
		Inbox:         inbox,
		LastHeartbeat: time.Now(),
	}

	return rn
}

func (rn *RaftNode) Start() {
	go rn.run()
	go rn.applyLoop()
}
