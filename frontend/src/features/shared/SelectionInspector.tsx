import { formatDateTime, formatUSD, prettyJSON, shortHash } from "../../lib/format";
import { nodeAddress, type GraphSelection } from "../../lib/graph";

interface SelectionInspectorProps {
  selection: GraphSelection;
  emptyMessage: string;
  onLookupTx?: (txID: string) => void;
}

function stringValues(value: unknown): string[] {
  if (Array.isArray(value)) {
    return Array.from(new Set(value.map((item) => String(item || "").trim().toUpperCase()).filter(Boolean)));
  }
  const single = String(value || "").trim().toUpperCase();
  return single ? [single] : [];
}

export function SelectionInspector({ selection, emptyMessage, onLookupTx }: SelectionInspectorProps) {
  if (!selection) {
    return <div className="empty-state">{emptyMessage}</div>;
  }

  if (selection.kind === "nodes") {
    const addresses = selection.nodes.map((node) => nodeAddress(node)).filter(Boolean);
    const chains = Array.from(new Set(selection.nodes.map((node) => node.chain).filter(Boolean)));
    const sourceProtocols = Array.from(
      new Set(
        selection.nodes.flatMap((node) =>
          stringValues(node.metrics?.source_protocols).concat(stringValues(node.metrics?.source_protocol))
        )
      )
    );
    return (
      <div className="detail-stack">
        <div>
          <span className="eyebrow">Nodes</span>
          <h3>{selection.nodes.length} selected</h3>
        </div>
        <dl className="detail-list">
          <div>
            <dt>Chains</dt>
            <dd>{chains.length ? chains.join(", ") : "n/a"}</dd>
          </div>
          <div>
            <dt>Addresses</dt>
            <dd>{addresses.length}</dd>
          </div>
          <div>
            <dt>Source Protocols</dt>
            <dd>{sourceProtocols.length ? sourceProtocols.join(", ") : "n/a"}</dd>
          </div>
        </dl>
        <div>
          <span className="section-label">Selected Nodes</span>
          <pre className="json-panel">
            {prettyJSON(
              selection.nodes.map((node) => ({
                id: node.id,
                label: node.label,
                kind: node.kind,
                chain: node.chain,
                address: nodeAddress(node),
              }))
            )}
          </pre>
        </div>
      </div>
    );
  }

  if (selection.kind === "node") {
    const address = nodeAddress(selection.node);
    const sourceProtocols = Array.from(
      new Set(stringValues(selection.node.metrics?.source_protocols).concat(stringValues(selection.node.metrics?.source_protocol)))
    );
    return (
      <div className="detail-stack">
        <div>
          <span className="eyebrow">Node</span>
          <h3>{selection.node.label}</h3>
        </div>
        <dl className="detail-list">
          <div>
            <dt>Kind</dt>
            <dd>{selection.node.kind}</dd>
          </div>
          <div>
            <dt>Chain</dt>
            <dd>{selection.node.chain || "n/a"}</dd>
          </div>
          <div>
            <dt>Depth</dt>
            <dd>{selection.node.depth}</dd>
          </div>
          <div>
            <dt>Address</dt>
            <dd className="mono-wrap">{address || "n/a"}</dd>
          </div>
          <div>
            <dt>Actor IDs</dt>
            <dd>{selection.node.actor_ids.length ? selection.node.actor_ids.join(", ") : "None"}</dd>
          </div>
          <div>
            <dt>Source Protocols</dt>
            <dd>{sourceProtocols.length ? sourceProtocols.join(", ") : "n/a"}</dd>
          </div>
        </dl>
        <div>
          <span className="section-label">Metrics</span>
          <pre className="json-panel">{prettyJSON(selection.node.metrics ?? {})}</pre>
        </div>
      </div>
    );
  }

  const edge = selection.edge;
  const sourceProtocols = Array.from(
    new Set(stringValues(edge.source_protocols).concat(stringValues(edge.inspect?.source_protocols)))
  );

  return (
    <div className="detail-stack">
      <div>
        <span className="eyebrow">Edge</span>
        <h3>{edge.action_label || edge.action_class}</h3>
      </div>
      <dl className="detail-list">
        <div>
          <dt>Action Class</dt>
          <dd>{edge.action_class}</dd>
        </div>
        <div>
          <dt>Domain</dt>
          <dd>{edge.action_domain || "n/a"}</dd>
        </div>
        <div>
          <dt>From</dt>
          <dd className="mono-wrap">{edge.from}</dd>
        </div>
        <div>
          <dt>To</dt>
          <dd className="mono-wrap">{edge.to}</dd>
        </div>
        <div>
          <dt>USD</dt>
          <dd>{formatUSD(edge.usd_spot)}</dd>
        </div>
        <div>
          <dt>Transactions</dt>
          <dd>{edge.tx_ids.length}</dd>
        </div>
        <div>
          <dt>Source Protocols</dt>
          <dd>{sourceProtocols.length ? sourceProtocols.join(", ") : "n/a"}</dd>
        </div>
      </dl>
      <div className="chip-list">
        {edge.tx_ids.slice(0, 6).map((txID) => (
          <button
            key={txID}
            type="button"
            className="chip-button"
            onClick={() => onLookupTx?.(txID)}
            disabled={!onLookupTx}
          >
            {shortHash(txID)}
          </button>
        ))}
      </div>
      <div>
        <span className="section-label">Transactions</span>
        <div className="table-wrap compact">
          <table className="data-table compact">
            <thead>
              <tr>
                <th>Time</th>
                <th>Source</th>
                <th>TX</th>
                <th>USD</th>
              </tr>
            </thead>
            <tbody>
              {edge.transactions.map((transaction) => (
                <tr key={`${edge.id}:${transaction.source_protocol || "THOR"}:${transaction.tx_id}`}>
                  <td>{formatDateTime(transaction.time)}</td>
                  <td>{transaction.source_protocol || "THOR"}</td>
                  <td>
                    <button
                      type="button"
                      className="table-link"
                      onClick={() => onLookupTx?.(transaction.tx_id)}
                      disabled={!onLookupTx}
                    >
                      {shortHash(transaction.tx_id)}
                    </button>
                  </td>
                  <td>{formatUSD(transaction.usd_spot)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div>
        <span className="section-label">Assets</span>
        <pre className="json-panel">{prettyJSON(edge.assets)}</pre>
      </div>
    </div>
  );
}
