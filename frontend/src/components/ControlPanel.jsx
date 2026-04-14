
import React from 'react';
import { Play, Square, Send, Zap } from 'lucide-react';

const ControlPanel = ({ nodes, onStop, onStart, onSubmit }) => {
    return (
        <div className="control-panel panel">
            <h3><Zap size={14} /> Chaos Control</h3>

            <div className="global-controls">
                <button className="btn btn-primary" onClick={onSubmit}>
                    <Send size={14} /> Send Work Request
                </button>
            </div>

            <div className="node-controls">
                {nodes.map(node => (
                    <div key={node.id} className="node-control-item">
                        <span className="node-label">
                            <span className={`node-dot ${node.is_alive ? 'alive' : ''}`} />
                            Node {node.id}
                        </span>
                        <div className="button-group">
                            {node.is_alive ? (
                                <button className="btn btn-danger" onClick={() => onStop(node.id)}>
                                    <Square size={14} /> Kill
                                </button>
                            ) : (
                                <button className="btn btn-success" onClick={() => onStart(node.id)}>
                                    <Play size={14} /> Start
                                </button>
                            )}
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
};

export default ControlPanel;
