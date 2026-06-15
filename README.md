# Raft Lab

An interactive simulator for the Raft consensus algorithm. It runs a 5-node cluster over an in-memory transport (with simulated message drops and latency) and streams live state to a web UI, so you can watch leader elections and log replication happen in real time — and trigger failure by crashing and reviving nodes from the browser.

## Features

- **Live cluster visualization** — node states (`FOLLOWER` · `CANDIDATE` · `LEADER`), terms, and the replicated log update in real time.
- **Leader election & replication** — a faithful implementation of Raft's election and log-replication flows.
- **Failure simulation** — crash and revive nodes, submit commands, and watch the cluster recover.
- **Virtual transport** — in-memory message passing with configurable drop rate and latency. No database; pure in-memory state.

## Requirements

- Go 1.23+
- Node.js 18+

## Getting started

```bash
# 1. Install frontend dependencies
cd frontend && npm install && cd ..

# 2. Run backend + frontend together
make run
```

Then open:

- **Frontend** → http://localhost:5173
- **Backend** → http://localhost:8080

### Running services individually

```bash
make run-backend    # Go backend on :8080
make run-frontend   # Vite frontend on :5173
```


