import { formatDateTime, formatUSD, shortHash } from "../../lib/format";
import type { SupportingAction } from "../../lib/types";

interface SupportingActionsTableProps {
  actions: SupportingAction[];
  onLookup: (txID: string) => void;
}

export function SupportingActionsTable({ actions, onLookup }: SupportingActionsTableProps) {
  if (!actions.length) {
    return <div className="empty-table">No supporting actions returned.</div>;
  }

  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Time</th>
            <th>Action</th>
            <th>Source</th>
            <th>TX</th>
            <th>Asset</th>
            <th>Amount</th>
            <th>USD</th>
          </tr>
        </thead>
        <tbody>
          {actions.map((action) => (
            <tr key={[action.source_protocol || "", action.tx_id, action.action_key, action.from_node, action.to_node].join("|")}>
              <td>{formatDateTime(action.time)}</td>
              <td>{action.action_label || action.action_class}</td>
              <td>{action.source_protocol || "THOR"}</td>
              <td>
                <button type="button" className="table-link" onClick={() => onLookup(action.tx_id)}>
                  {shortHash(action.tx_id)}
                </button>
              </td>
              <td>{action.primary_asset || "n/a"}</td>
              <td>{action.amount_raw || "n/a"}</td>
              <td>{formatUSD(action.usd_spot)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
