import { prettyJSON, shortHash } from "../../lib/format";
import type { ActionLookupResponse } from "../../lib/types";

interface ActionLookupPanelProps {
  result: ActionLookupResponse | null;
  isLoading: boolean;
  error: string;
}

export function ActionLookupPanel({ result, isLoading, error }: ActionLookupPanelProps) {
  if (isLoading) {
    return <div className="empty-state">Looking up action details…</div>;
  }

  if (error) {
    return <p className="error-text">{error}</p>;
  }

  if (!result) {
    return <div className="empty-state">Select a transaction from the graph or action table to inspect Midgard action data.</div>;
  }

  return (
    <div className="detail-stack">
      <div>
        <span className="eyebrow">Lookup</span>
        <h3>{shortHash(result.tx_id)}</h3>
      </div>
      <p className="section-note">{result.actions.length} canonical action(s) returned for this transaction.</p>
      <div className="table-wrap compact">
        <table className="data-table compact">
          <thead>
            <tr>
              <th>Source</th>
              <th>Type</th>
              <th>Height</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {result.actions.map((action, index) => (
              <tr key={`${result.tx_id}:${action.source_protocol || "THOR"}:${action.type || "action"}:${action.height || index}`}>
                <td>{action.source_protocol || "THOR"}</td>
                <td>{String(action.type || "n/a")}</td>
                <td>{String(action.height || "n/a")}</td>
                <td>{String(action.status || "n/a")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <pre className="json-panel">{prettyJSON(result.actions)}</pre>
    </div>
  );
}
