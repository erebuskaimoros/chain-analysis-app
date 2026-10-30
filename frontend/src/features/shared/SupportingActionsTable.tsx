import { formatShortDateTime, formatUSD, middleTruncate } from "../../lib/format";
import type { SupportingAction } from "../../lib/types";
import { MenuButton } from "../../ui/Menu";

interface SupportingActionsTableProps {
  actions: SupportingAction[];
  onLookup: (txID: string) => void;
  onAddToCase?: (action: SupportingAction) => void;
  selectedTxID?: string;
}

// Every action that supports the graph's edges. Clicking a row looks the
// transaction up in Midgard.
export function SupportingActionsTable({ actions, onLookup, onAddToCase, selectedTxID }: SupportingActionsTableProps) {
  if (!actions.length) {
    return <div className="empty-table">No supporting actions for the current graph and filters.</div>;
  }

  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Time</th>
            <th>Action</th>
            <th>Source</th>
            <th>Transaction</th>
            <th>Asset</th>
            <th className="numeric">Amount</th>
            <th className="numeric">Value at the time</th>
            {onAddToCase ? <th className="cell-actions" aria-label="Row actions" /> : null}
          </tr>
        </thead>
        <tbody>
          {actions.map((action) => (
            <tr
              key={[action.source_protocol || "", action.tx_id, action.action_key, action.from_node, action.to_node].join("|")}
              className={`row-clickable${selectedTxID === action.tx_id ? " is-selected" : ""}`}
              onClick={() => onLookup(action.tx_id)}
            >
              <td className="cell-muted">{formatShortDateTime(action.time)}</td>
              <td>{action.action_label || action.action_class}</td>
              <td>{action.source_protocol || "THOR"}</td>
              <td>
                <button
                  type="button"
                  className="link-btn mono"
                  title={`Look up ${action.tx_id}`}
                  onClick={(event) => {
                    event.stopPropagation();
                    onLookup(action.tx_id);
                  }}
                >
                  {middleTruncate(action.tx_id, 8, 6)}
                </button>
              </td>
              <td>{action.primary_asset || "n/a"}</td>
              <td className="numeric">{action.amount_raw || "n/a"}</td>
              <td className="numeric" title={action.price_source ? `Priced from ${action.price_source}` : undefined}>
                {formatUSD(action.usd_spot)}
              </td>
              {onAddToCase ? (
                <td className="cell-actions" onClick={(event) => event.stopPropagation()}>
                  <MenuButton
                    label={`Actions for ${action.tx_id}`}
                    items={[
                      { label: "Look up transaction", onSelect: () => onLookup(action.tx_id) },
                      { label: "Add to the active case", onSelect: () => onAddToCase(action) },
                      {
                        label: "Copy transaction hash",
                        onSelect: () => void navigator.clipboard?.writeText(action.tx_id),
                      },
                    ]}
                  />
                </td>
              ) : null}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
