
import React from 'react';
import { Crown, Activity, HeartPulse, Skull, Info } from 'lucide-react';
import { STATE_COLORS } from './RaftyVisualizer';

const ITEMS = [
    { label: 'Leader', desc: 'Sends heartbeats & replicates entries', color: STATE_COLORS.LEADER, Icon: Crown },
    { label: 'Candidate', desc: 'Requesting votes for a new term', color: STATE_COLORS.CANDIDATE, Icon: Activity },
    { label: 'Follower', desc: 'Replicates the leader’s log', color: STATE_COLORS.FOLLOWER, Icon: HeartPulse },
    { label: 'Offline', desc: 'Stopped — not participating', color: STATE_COLORS.DEAD, Icon: Skull },
];

const StateLegend = () => {
    return (
        <div className="state-legend panel">
            <h3><Info size={14} /> Node States</h3>
            <div className="legend-list">
                {ITEMS.map(({ label, desc, color, Icon }) => (
                    <div key={label} className="legend-row">
                        <span
                            className="legend-badge"
                            style={{ color, background: `${color}1f`, borderColor: `${color}55` }}
                        >
                            <Icon size={14} />
                        </span>
                        <div className="legend-text">
                            <span className="legend-label" style={{ color }}>{label}</span>
                            <span className="legend-desc">{desc}</span>
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
};

export default StateLegend;
