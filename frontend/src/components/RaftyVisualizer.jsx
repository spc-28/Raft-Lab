
import React from 'react';
import { motion } from 'framer-motion';
import { Crown, HeartPulse, Skull, Activity } from 'lucide-react';

export const STATE_COLORS = {
    LEADER: '#34d399',
    CANDIDATE: '#fbbf24',
    FOLLOWER: '#818cf8',
    DEAD: '#fb7185',
};

const RaftyVisualizer = ({ state }) => {
    if (!state) return <div className="loading">Waiting for simulation data…</div>;

    const { nodes } = state;

    // Calculate positions on a circle
    const radius = 210;
    const centerX = 300;
    const centerY = 250;

    const getNodePos = (index, total) => {
        const angle = (index * (360 / total) - 90) * (Math.PI / 180);
        return {
            x: centerX + radius * Math.cos(angle),
            y: centerY + radius * Math.sin(angle),
        };
    };

    // Connections behind nodes
    const connections = [];
    for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
            const p1 = getNodePos(i, nodes.length);
            const p2 = getNodePos(j, nodes.length);
            connections.push(
                <line
                    key={`${i}-${j}`}
                    x1={p1.x} y1={p1.y}
                    x2={p2.x} y2={p2.y}
                    stroke="rgba(255,255,255,0.05)"
                    strokeWidth="1.5"
                    strokeDasharray="3,7"
                    strokeLinecap="round"
                />
            );
        }
    }

    // Leader → follower heartbeat pulses (animated traveling dots)
    const leader = nodes.find(n => n.state === 'LEADER' && n.is_alive);
    const rpcLines = [];
    const rpcPulses = [];
    if (leader) {
        const leaderIndex = nodes.findIndex(n => n.id === leader.id);
        const lp = getNodePos(leaderIndex, nodes.length);

        nodes.forEach((node, idx) => {
            if (node.id !== leader.id && node.is_alive) {
                const np = getNodePos(idx, nodes.length);
                rpcLines.push(
                    <line
                        key={`rpc-line-${leader.id}-${node.id}`}
                        x1={lp.x} y1={lp.y}
                        x2={np.x} y2={np.y}
                        stroke="rgba(52,211,153,0.18)"
                        strokeWidth="1.5"
                    />
                );
                rpcPulses.push(
                    <motion.circle
                        key={`rpc-pulse-${leader.id}-${node.id}`}
                        r="3.5"
                        fill="#34d399"
                        filter="url(#glow)"
                        initial={{ cx: lp.x, cy: lp.y, opacity: 0 }}
                        animate={{ cx: [lp.x, np.x], cy: [lp.y, np.y], opacity: [0, 1, 1, 0] }}
                        transition={{ duration: 1, repeat: Infinity, repeatDelay: 0.6, ease: 'easeInOut' }}
                    />
                );
            }
        });
    }

    return (
        <div className="rafty-visualizer" style={{ width: '100%', height: '100%', position: 'relative', display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
            <svg width="100%" height="100%" viewBox="0 0 600 500" preserveAspectRatio="xMidYMid meet" style={{ maxWidth: '100%', maxHeight: '100%' }}>
                <defs>
                    <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
                        <feGaussianBlur stdDeviation="3" result="blur" />
                        <feMerge>
                            <feMergeNode in="blur" />
                            <feMergeNode in="SourceGraphic" />
                        </feMerge>
                    </filter>
                </defs>

                {/* Network topology */}
                <g className="connections">{connections}</g>
                <g className="rpc-lines">{rpcLines}</g>
                <g className="rpc-pulses">{rpcPulses}</g>

                {/* Nodes */}
                {nodes.map((node, index) => {
                    const pos = getNodePos(index, nodes.length);
                    const isLeader = node.state === 'LEADER';
                    const isCandidate = node.state === 'CANDIDATE';
                    const isDead = !node.is_alive;

                    const color = isDead
                        ? STATE_COLORS.DEAD
                        : isLeader
                            ? STATE_COLORS.LEADER
                            : isCandidate
                                ? STATE_COLORS.CANDIDATE
                                : STATE_COLORS.FOLLOWER;

                    const stateLabel = isDead ? 'OFFLINE' : node.state;

                    return (
                        <g key={node.id}>
                            {/* Pulsing halo for leader */}
                            {isLeader && (
                                <motion.circle
                                    cx={pos.x} cy={pos.y}
                                    r={38}
                                    stroke={color}
                                    strokeWidth="1.5"
                                    fill="none"
                                    animate={{ r: [33, 50, 33], opacity: [0.5, 0, 0.5] }}
                                    transition={{ duration: 2, repeat: Infinity, ease: 'easeInOut' }}
                                />
                            )}

                            {/* Soft glow disc */}
                            <circle cx={pos.x} cy={pos.y} r={36} fill={color} opacity={isDead ? 0.05 : 0.13} />

                            {/* Node body */}
                            <motion.circle
                                cx={pos.x} cy={pos.y}
                                r={28}
                                fill="rgba(18,19,26,0.95)"
                                stroke={color}
                                strokeWidth="2"
                                strokeDasharray={isDead ? '3,4' : '0'}
                                initial={{ scale: 0 }}
                                animate={{ scale: 1 }}
                                transition={{ type: 'spring', stiffness: 260, damping: 20 }}
                            />

                            {/* Icon */}
                            <foreignObject x={pos.x - 11} y={pos.y - 11} width="22" height="22" style={{ pointerEvents: 'none' }}>
                                <div style={{ color, display: 'flex', justifyContent: 'center', opacity: isDead ? 0.7 : 1 }}>
                                    {isDead ? <Skull size={20} /> :
                                        isLeader ? <Crown size={20} /> :
                                            isCandidate ? <Activity size={20} /> :
                                                <HeartPulse size={20} />}
                                </div>
                            </foreignObject>

                            {/* State pill */}
                            <g>
                                <rect
                                    x={pos.x - 30} y={pos.y + 36}
                                    width={60} height={17} rx={8.5}
                                    fill={`${color}22`}
                                    stroke={`${color}55`}
                                    strokeWidth="1"
                                />
                                <text x={pos.x} y={pos.y + 47.5} textAnchor="middle" fill={color} fontSize="8.5" fontWeight="700" letterSpacing="0.08em" fontFamily="Inter, sans-serif">{stateLabel}</text>
                            </g>

                            {/* Labels */}
                            <text x={pos.x} y={pos.y + 68} textAnchor="middle" fill="#e8e9ee" fontSize="12.5" fontWeight="600" fontFamily="Inter, sans-serif">Node {node.id}</text>
                            <text x={pos.x} y={pos.y + 81} textAnchor="middle" fill="#5e6070" fontSize="9.5" fontFamily="'JetBrains Mono', monospace">term {node.current_term}</text>
                        </g>
                    );
                })}
            </svg>
        </div>
    );
};

export default RaftyVisualizer;
