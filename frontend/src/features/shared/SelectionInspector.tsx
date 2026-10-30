import type { ReactNode } from "react";
import { formatDateTime, formatUSD, pluralize, prettyJSON } from "../../lib/format";
import { nodeAddress, type GraphSelection, type VisibleGraphNode } from "../../lib/graph";
import { AddressText, ChainBadge } from "../../ui/AddressText";

interface SelectionInspectorProps {
  selection: GraphSelection;
  emptyMessage: string;
  onLookupTx?: (txID: string) => void;
  // Resolves graph node IDs (edge endpoints) to readable names.
  nodeLabel?: (id: string) => string;
  actorName?: (id: number) => string;
  actions?: ReactNode;
}

const KIND_LABELS: Record<string, string> = {
  actor: "Actor",
  actor_address: "Actor address",
  external_address: "External address",
  external_cluster: "External addresses on one chain",
  explorer_target: "Explored address",
  pool: "Liquidity pool",
  node: "Validator node",
  inbound: "Inbound vault",
  router: "Router contract",
};

export function kindLabel(kind: string) {
  return KIND_LABELS[kind] ?? kind.replace(/_/g, " ").replace(/^\w/, (letter) => letter.toUpperCase());
}

function stringValues(value: unknown): string[] {
  if (Array.isArray(value)) {
    return Array.from(new Set(value.map((item) => String(item || "").trim().toUpperCase()).filter(Boolean)));
  }
  const single = String(value || "").trim().toUpperCase();
  return single ? [single] : [];
}

function RawData({ value }: { value: unknown }) {
  return (
    <details className="raw-data">
      <summary>Raw data</summary>
      <pre className="json-panel">{prettyJSON(value)}</pre>
    </details>
  );
}

export function SelectionInspector({ selection, emptyMessage, onLookupTx, nodeLabel, actorName, actions }: SelectionInspectorProps) {
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
      <div className="section">
        <div className="inspector-title">
          <h3>{selection.nodes.length} nodes selected</h3>
          <div className="inspector-kind">
            {chains.map((chain) => (
              <ChainBadge key={chain} chain={chain} />
            ))}
          </div>
        </div>
        {actions ? <div className="inspector-actions">{actions}</div> : null}
        <dl className="detail-list">
          <dt>Addresses</dt>
          <dd>{addresses.length}</dd>
          <dt>Source protocols</dt>
          <dd>{sourceProtocols.length ? sourceProtocols.join(", ") : "n/a"}</dd>
        </dl>
        <ul className="list ws-run-list">
          {selection.nodes.map((node) => (
            <li key={node.id} className="list-row">
              <span className="list-row-main">
                <span className="list-row-title">{node.displayLabel || node.label}</span>
                <span className="list-row-meta">{kindLabel(node.kind)}</span>
              </span>
              {nodeAddress(node) ? <AddressText value={nodeAddress(node)} head={6} tail={4} /> : null}
            </li>
          ))}
        </ul>
      </div>
    );
  }

  if (selection.kind === "node") {
    return <NodeDetails node={selection.node} actions={actions} actorName={actorName} />;
  }

  const edge = selection.edge;
  const sourceProtocols = Array.from(
    new Set(stringValues(edge.source_protocols).concat(stringValues(edge.inspect?.source_protocols)))
  );
  const fromLabel = nodeLabel?.(edge.source) || edge.from;
  const toLabel = nodeLabel?.(edge.target) || edge.to;

  return (
    <div className="section">
      <div className="inspector-title">
        <h3>{edge.action_label || edge.action_class}</h3>
        <div className="inspector-kind">
          <span>{fromLabel}</span>
          <span aria-label="to">→</span>
          <span>{toLabel}</span>
        </div>
      </div>
      {actions ? <div className="inspector-actions">{actions}</div> : null}
      <dl className="detail-list">
        <dt>{typeof edge.usd_now === "number" ? "Value at the time" : "Value"}</dt>
        <dd className="num">{formatUSD(edge.usd_spot)}</dd>
        {typeof edge.usd_now === "number" ? (
          <>
            <dt>Value today</dt>
            <dd className="num">{formatUSD(edge.usd_now)}</dd>
          </>
        ) : null}
        <dt>Transactions</dt>
        <dd>{edge.tx_ids.length}</dd>
        <dt>Action class</dt>
        <dd>{edge.action_class}</dd>
        {edge.action_domain ? (
          <>
            <dt>Domain</dt>
            <dd>{edge.action_domain}</dd>
          </>
        ) : null}
        <dt>Source protocols</dt>
        <dd>{sourceProtocols.length ? sourceProtocols.join(", ") : "n/a"}</dd>
      </dl>
      {edge.assets.length ? (
        <div className="section">
          <h4 className="section-title">Assets</h4>
          <ul className="label-list">
            {edge.assets.map((asset, index) => (
              <li key={`${asset.asset}:${index}`} className="num">
                {asset.amount_raw} {asset.asset}
                {asset.usd_spot ? <span className="label-source"> {formatUSD(asset.usd_spot)}</span> : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <div className="section">
        <h4 className="section-title">Transactions</h4>
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>Source</th>
                <th>Transaction</th>
                <th className="numeric">Value</th>
              </tr>
            </thead>
            <tbody>
              {edge.transactions.map((transaction) => (
                <tr key={`${edge.id}:${transaction.source_protocol || "THOR"}:${transaction.tx_id}`}>
                  <td>{formatDateTime(transaction.time)}</td>
                  <td>{transaction.source_protocol || "THOR"}</td>
                  <td>
                    {onLookupTx ? (
                      <button
                        type="button"
                        className="link-btn mono"
                        title={`Look up ${transaction.tx_id}`}
                        onClick={() => onLookupTx(transaction.tx_id)}
                      >
                        {transaction.tx_id.slice(0, 8)}…{transaction.tx_id.slice(-6)}
                      </button>
                    ) : (
                      <AddressText value={transaction.tx_id} />
                    )}
                  </td>
                  <td className="numeric">{formatUSD(transaction.usd_spot)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <RawData value={{ assets: edge.assets, inspect: edge.inspect }} />
    </div>
  );
}

function NodeDetails({
  node,
  actions,
  actorName,
}: {
  node: VisibleGraphNode;
  actions?: ReactNode;
  actorName?: (id: number) => string;
}) {
  const address = nodeAddress(node);
  const sourceProtocols = Array.from(
    new Set(stringValues(node.metrics?.source_protocols).concat(stringValues(node.metrics?.source_protocol)))
  );
  const labels = nodeLabels(node);
  const flowUSD = Number(node.metrics?.usd_spot ?? 0);

  return (
    <div className="section">
      <div className="inspector-title">
        <h3>{node.displayLabel || node.label}</h3>
        <div className="inspector-kind">
          <span>{kindLabel(node.kind)}</span>
          {node.chain ? <ChainBadge chain={node.chain} /> : null}
          <span>{node.depth === 0 ? "Seed" : `Hop ${node.depth}`}</span>
        </div>
      </div>
      {actions ? <div className="inspector-actions">{actions}</div> : null}
      {labels.length ? (
        <ul className="label-list">
          {labels.map((entry) => (
            <li key={`${entry.source}:${entry.label}`}>
              {entry.category ? (
                <span className={`label-category-badge label-category-${entry.category}`}>{entry.category.replace(/_/g, " ")}</span>
              ) : null}{" "}
              {entry.label}{" "}
              <span className="label-source">
                {entry.source}, {Math.round(entry.confidence * 100) / 100}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      <dl className="detail-list">
        <dt>Address</dt>
        <dd>{address ? <AddressText value={address} full /> : "n/a"}</dd>
        {node.live_holdings_label ? (
          <>
            <dt>Holds now</dt>
            <dd>{node.live_holdings_label}</dd>
          </>
        ) : null}
        {flowUSD > 0 ? (
          <>
            <dt>Flow in graph</dt>
            <dd className="num">{formatUSD(flowUSD)}</dd>
          </>
        ) : null}
        <dt>Actors</dt>
        <dd>
          {node.actor_ids.length
            ? node.actor_ids.map((id) => actorName?.(id) || `Actor ${id}`).join(", ")
            : "None"}
        </dd>
        <dt>Source protocols</dt>
        <dd>{sourceProtocols.length ? sourceProtocols.join(", ") : "n/a"}</dd>
        {node.raw_node_ids.length > 1 ? (
          <>
            <dt>Grouped</dt>
            <dd>{pluralize(node.raw_node_ids.length, "address", "addresses")}</dd>
          </>
        ) : null}
      </dl>
      <RawData value={node.metrics ?? {}} />
    </div>
  );
}

interface NodeLabelEntry {
  label: string;
  category: string;
  source: string;
  confidence: number;
}

// nodeLabels reads the server-provided label attributions, best first.
function nodeLabels(node: { metrics: Record<string, unknown> | null }): NodeLabelEntry[] {
  const raw = node.metrics?.labels;
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw
    .filter((entry): entry is Record<string, unknown> => Boolean(entry) && typeof entry === "object")
    .map((entry) => ({
      label: String(entry.label ?? ""),
      category: String(entry.category ?? ""),
      source: String(entry.source ?? ""),
      confidence: Number(entry.confidence ?? 0),
    }))
    .filter((entry) => entry.label);
}
