
import React, { useState, useEffect, useRef } from 'react';
import RaftyVisualizer from './components/RaftyVisualizer';
import ControlPanel from './components/ControlPanel';
import LogStream from './components/LogStream';
import ReplicatedLog from './components/ReplicatedLog';
import StateLegend from './components/StateLegend';
import axios from 'axios';
import { Network, Crown, Hash, Server, Layers } from 'lucide-react';
import './App.css';

const API_URL = 'http://localhost:8080';
const WS_URL = 'ws://localhost:8080/ws';

function App() {
  const [clusterState, setClusterState] = useState(null);
  const [logs, setLogs] = useState([]);
  const wsRef = useRef(null);

  const connectWebSocket = () => {
    wsRef.current = new WebSocket(WS_URL);

    wsRef.current.onopen = () => {
      console.log('Connected to WebSocket');
      addLog('System', 'Connected to simulation backend');
    };

    wsRef.current.onmessage = (event) => {
      const state = JSON.parse(event.data);
      setClusterState(state);
    };

    wsRef.current.onclose = () => {
      console.log('Disconnected from WebSocket');
      addLog('System', 'Disconnected from backend. Retrying in 2s...');
      setTimeout(connectWebSocket, 2000);
    };
  };

  useEffect(() => {
    connectWebSocket();

    return () => {
      if (wsRef.current) {
        wsRef.current.onclose = null; // Prevent retry on unmount
        wsRef.current.close();
      }
    };
  }, []);

  const addLog = (source, message) => {
    setLogs(prev => [{
      id: Date.now() + Math.random(),
      time: new Date().toLocaleTimeString(),
      source,
      message
    }, ...prev].slice(0, 50));
  };

  // Diffing for logs
  const prevStateRef = useRef(null);
  useEffect(() => {
    if (!clusterState || !prevStateRef.current) {
      prevStateRef.current = clusterState;
      return;
    }

    // Check for leader changes
    const prevLeader = prevStateRef.current.nodes.find(n => n.state === 'LEADER');
    const currLeader = clusterState.nodes.find(n => n.state === 'LEADER');

    if (currLeader && (!prevLeader || prevLeader.id !== currLeader.id)) {
      addLog('Election', `Node ${currLeader.id} elected Leader for Term ${currLeader.current_term}`);
    }

    // Check for state changes
    clusterState.nodes.forEach(node => {
      const prevNode = prevStateRef.current.nodes.find(n => n.id === node.id);
      if (!prevNode) return;

      if (node.current_term > prevNode.current_term) {
        addLog(`Node ${node.id}`, `Term increased to ${node.current_term}`);
      }
      if (node.state !== prevNode.state) {
        // addLog(`Node ${node.id}`, `State changed to ${node.state}`);
      }
    });

    prevStateRef.current = clusterState;
  }, [clusterState]);

  const handleStopNode = async (id) => {
    try {
      await axios.post(`${API_URL}/control/stop/${id}`);
      addLog('User', `Stopped Node ${id}`);
    } catch (err) {
      console.error(err);
    }
  };

  const handleStartNode = async (id) => {
    try {
      await axios.post(`${API_URL}/control/start/${id}`);
      addLog('User', `Started Node ${id}`);
    } catch (err) {
      console.error(err);
    }
  };

  const handleSubmitWork = async () => {
    try {
      const cmd = `CMD-${Math.floor(Math.random() * 1000)}`;
      await axios.post(`${API_URL}/control/submit`, { command: cmd });
      addLog('User', `Submitted command: ${cmd}`);
    } catch (err) {
      addLog('Error', 'Failed to submit work (No Leader?)');
    }
  };

  const nodes = clusterState?.nodes || [];
  const leader = nodes.find(n => n.state === 'LEADER' && n.is_alive);
  const currentTerm = nodes.reduce((max, n) => Math.max(max, n.current_term || 0), 0);
  const onlineCount = nodes.filter(n => n.is_alive).length;
  const logEntries = nodes.reduce((max, n) => Math.max(max, n.log?.length || 0), 0);

  const stats = [
    { icon: Crown, label: 'Leader', value: leader ? `Node ${leader.id}` : 'None', accent: leader ? 'leader' : 'muted' },
    { icon: Hash, label: 'Current Term', value: currentTerm },
    { icon: Server, label: 'Nodes Online', value: `${onlineCount} / ${nodes.length || 0}` },
    { icon: Layers, label: 'Log Entries', value: logEntries },
  ];

  return (
    <div className="app-container">
      <header className="app-header">
        <h1>
          <span className="brand-mark"><Network size={18} /></span>
          <span>
            Raft Consensus Simulator
            <span className="brand-sub">Distributed Systems Lab</span>
          </span>
        </h1>
        <div className="status-badge">
          {clusterState
            ? <span className="online"><span className="dot" /> System Online</span>
            : <span className="offline"><span className="dot" /> Connecting…</span>}
        </div>
      </header>

      <div className="stats-bar">
        {stats.map(({ icon: Icon, label, value, accent }) => (
          <div key={label} className="stat-card">
            <span className={`stat-icon ${accent || ''}`}><Icon size={16} /></span>
            <div className="stat-meta">
              <span className="stat-label">{label}</span>
              <span className="stat-value">{value}</span>
            </div>
          </div>
        ))}
      </div>

      <div className="main-content">
        <section className="visualization-area">
          <div className="section-tag">Cluster Topology</div>
          <RaftyVisualizer state={clusterState} />
        </section>

        <section className="middle-panel">
          <div className="stack-top">
            <ReplicatedLog nodes={nodes} />
          </div>
          <div className="stack-bottom">
            <LogStream logs={logs} />
          </div>
        </section>

        <aside className="sidebar">
          <ControlPanel
            nodes={nodes}
            onStop={handleStopNode}
            onStart={handleStartNode}
            onSubmit={handleSubmitWork}
          />
          <StateLegend />
        </aside>
      </div>
    </div>
  );
}

export default App;
