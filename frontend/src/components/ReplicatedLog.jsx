
import React from 'react';
import { Layers } from 'lucide-react';

const ReplicatedLog = ({ nodes }) => {
    return (
        <div className="replicated-log panel">
            <h3><Layers size={14} /> Replicated Log <span className="count-pill">{nodes.length} nodes</span></h3>
            <div className="log-grid">
                {nodes.map(node => {
                    // commit_index / last_applied default to -1 when absent.
                    const commitIndex = node.commit_index ?? -1;
                    const lastApplied = node.last_applied ?? -1;
                    return (
                    <div key={node.id} className={`log-row ${!node.is_alive ? 'dead' : ''}`}>
                        <div className="node-id-badge">N{node.id}</div>
                        <div className="entries">
                            {node.log && node.log.length > 0 ? (
                                node.log.map((entry, i) => {
                                    // index <= commit_index = committed (solid),
                                    // otherwise uncommitted (faded). The last_applied
                                    // position gets a distinct marker.
                                    const committed = i <= commitIndex;
                                    const applied = i === lastApplied;
                                    const cls = [
                                        'log-entry-block',
                                        committed ? 'committed' : 'uncommitted',
                                        applied ? 'applied' : '',
                                    ].join(' ').trim();
                                    return (
                                        <div key={i} className={cls} title={`index ${i} · term ${entry.term}${committed ? ' · committed' : ' · uncommitted'}${applied ? ' · applied' : ''}`}>
                                            <span className="term">{entry.term}</span>
                                        </div>
                                    );
                                })
                            ) : (
                                <span className="empty-log">Empty Log</span>
                            )}

                            {/* Visual filler for empty space if log is short */}
                            <div className="log-track-line"></div>
                        </div>
                    </div>
                    );
                })}
            </div>
        </div>
    );
};

export default ReplicatedLog;
