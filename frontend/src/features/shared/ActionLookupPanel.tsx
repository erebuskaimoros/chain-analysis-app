import { prettyJSON } from "../../lib/format";
import type { ActionLookupResponse } from "../../lib/types";
import { AddressText } from "../../ui/AddressText";

interface ActionLookupPanelProps {
  result: ActionLookupResponse | null;
  isLoading: boolean;
  error: string;
}

export function ActionLookupPanel({ result, isLoading, error }: ActionLookupPanelProps) {
  if (isLoading) {
    return <p className="status-text">Looking up the transaction in Midgard…</p>;
  }

  if (error) {
    return <p className="error-text">{error}</p>;
  }

  if (!result) {
    return (
      <div className="empty-state">
        Pick a transaction in the selection details or the supporting actions to see what Midgard recorded for it.
      </div>
    );
  }

  return (
    <div className="section">
      <div className="inspector-title">
        <h3>
          <AddressText value={result.tx_id} head={10} tail={8} />
        </h3>
        <div className="inspector-kind">
          {result.actions.length === 1 ? "1 action recorded" : `${result.actions.length} actions recorded`}
        </div>
      </div>
      <div className="table-wrap">
        <table className="data-table">
          <thead>
            <tr>
              <th>Source</th>
              <th>Type</th>
              <th className="numeric">Height</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {result.actions.map((action, index) => (
              <tr key={`${result.tx_id}:${action.source_protocol || "THOR"}:${action.type || "action"}:${action.height || index}`}>
                <td>{action.source_protocol || "THOR"}</td>
                <td>{String(action.type || "n/a")}</td>
                <td className="numeric">{String(action.height || "n/a")}</td>
                <td>{String(action.status || "n/a")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <details className="raw-data">
        <summary>Raw data</summary>
        <pre className="json-panel">{prettyJSON(result.actions)}</pre>
      </details>
    </div>
  );
}
