import { useMemo, useState } from "react";
import { formatShortDateTime, formatUSD, shortHash } from "../../lib/format";
import { chainForAsset, explorerURLForAddress, explorerURLForTx } from "../../lib/graph/actions";
import { deriveExplorerVisibleGraph } from "../../lib/graph/derive";
import { createGraphFilterState } from "../../lib/graph/filters";
import type { GraphSelection } from "../../lib/graph/types";
import type { AddressExplorerResponse, TraceAmount, TraceEndpoint, TraceResponse } from "../../lib/types";
import { GraphCanvas } from "../shared/GraphCanvas";
import { SelectionInspector } from "../shared/SelectionInspector";

const FLOW_ROW_LIMIT = 500;

const reasonLabels: Record<string, string> = {
  held: "Still held",
  held_before_window: "Held before the window",
  pool: "Liquidity pool",
  bond: "Validator bond",
  contract: "Contract",
  protocol: "Protocol address",
  max_depth: "Hop limit",
  max_branches: "Branch limit",
  below_min_usd: "Below minimum",
  expansion_limit: "Expansion limit",
  unexpanded: "Not expanded",
};

export function traceReasonLabel(reason: string) {
  return reasonLabels[reason] ?? reason.replace(/_/g, " ");
}

export function formatTraceAmount(amount: number) {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: amount >= 1000 ? 2 : amount >= 1 ? 4 : 8 }).format(amount);
}

function AssetList({ assets }: { assets: TraceAmount[] }) {
  return (
    <>
      {assets.map((asset) => (
        <div key={asset.asset}>
          {formatTraceAmount(asset.amount)} {asset.asset}
        </div>
      ))}
    </>
  );
}

function CategoryBadge({ category }: { category?: string }) {
  if (!category) {
    return null;
  }
  return <span className={`label-category-badge label-category-${category}`}>{category.replace(/_/g, " ")}</span>;
}

function TxLink({ txID, chain }: { txID: string; chain: string }) {
  const url = explorerURLForTx(txID, chain);
  if (!url) {
    return <span className="mono-wrap">{shortHash(txID)}</span>;
  }
  return (
    <a className="table-link mono-wrap" href={url} target="_blank" rel="noreferrer" title={txID}>
      {shortHash(txID)}
    </a>
  );
}

function EndpointTable({ title, endpoints, emptyText }: { title: string; endpoints: TraceEndpoint[]; emptyText: string }) {
  return (
    <section className="panel page-panel">
      <div className="panel-head">
        <div>
          <span className="eyebrow">{endpoints.length} endpoints</span>
          <h2>{title}</h2>
        </div>
      </div>
      {endpoints.length ? (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Why it stops</th>
                <th className="numeric">Traced</th>
                <th className="numeric">USD at time</th>
                <th className="numeric">Confidence</th>
                <th className="numeric">Holds now</th>
                <th className="numeric">Hop</th>
              </tr>
            </thead>
            <tbody>
              {endpoints.map((endpoint) => {
                const url = endpoint.address ? explorerURLForAddress(endpoint.address, endpoint.chain ?? "") : "";
                return (
                  <tr key={`${endpoint.node_id}|${endpoint.reason}`}>
                    <td>
                      <CategoryBadge category={endpoint.category} />{" "}
                      {url ? (
                        <a className="table-link" href={url} target="_blank" rel="noreferrer" title={endpoint.address}>
                          {endpoint.label}
                        </a>
                      ) : (
                        endpoint.label
                      )}
                      {endpoint.chain ? <span className="mono-wrap"> ({endpoint.chain})</span> : null}
                    </td>
                    <td>{traceReasonLabel(endpoint.reason)}</td>
                    <td className="numeric">
                      <AssetList assets={endpoint.assets} />
                    </td>
                    <td className="numeric">{endpoint.traced_usd_at_time ? formatUSD(endpoint.traced_usd_at_time) : "unpriced"}</td>
                    <td className="numeric">{Math.round(endpoint.confidence * 100)}%</td>
                    <td className="numeric">
                      {endpoint.holdings_usd != null ? formatUSD(endpoint.holdings_usd) : endpoint.holdings_status || "—"}
                    </td>
                    <td className="numeric">{endpoint.depth}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="empty-state">{emptyText}</p>
      )}
    </section>
  );
}

export function TraceResults({ result }: { result: TraceResponse }) {
  const [selection, setSelection] = useState<GraphSelection>(null);
  const backward = result.query.direction === "backward";
  const labels = useMemo(() => new Map(result.nodes.map((node) => [node.id, node.label || node.id])), [result.nodes]);
  const visibleGraph = useMemo(
    () =>
      deriveExplorerVisibleGraph({ nodes: result.nodes, edges: result.edges } as unknown as AddressExplorerResponse, createGraphFilterState(), {
        annotations: [],
        blocklist: [],
      }),
    [result.nodes, result.edges]
  );
  const flows = useMemo(
    () =>
      result.edges
        .flatMap((edge) => edge.traced_transactions.map((tx) => ({ edge, tx })))
        .sort((a, b) => new Date(a.tx.time).getTime() - new Date(b.tx.time).getTime()),
    [result.edges]
  );
  const gaps = result.coverage_gaps ?? [];
  const warnings = result.warnings ?? [];

  return (
    <div className="page-stack trace-results">
      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">{backward ? "Backward trace" : "Forward trace"}</span>
            <h2>Result</h2>
          </div>
        </div>
        <div className="trace-tiles">
          <div className="trace-tile">
            <span className="trace-tile-label">{backward ? "Traced into the seeds" : "Traced from the seeds"}</span>
            <strong className="trace-tile-value">{formatUSD(result.totals.seed_usd)}</strong>
            <span className="trace-tile-detail">
              <AssetList assets={result.totals.seed_assets ?? []} />
            </span>
          </div>
          <div className="trace-tile">
            <span className="trace-tile-label">{backward ? "Reached sources" : "Reached sinks"}</span>
            <strong className="trace-tile-value">{formatUSD(result.totals.sink_usd)}</strong>
            <span className="trace-tile-detail">{result.sinks.length} endpoints</span>
          </div>
          <div className="trace-tile">
            <span className="trace-tile-label">Stopped by limits</span>
            <strong className="trace-tile-value">{formatUSD(result.totals.frontier_usd)}</strong>
            <span className="trace-tile-detail">{result.frontier.length} endpoints</span>
          </div>
          <div className="trace-tile">
            <span className="trace-tile-label">Coverage gaps</span>
            <strong className="trace-tile-value">{gaps.length}</strong>
            <span className="trace-tile-detail">{gaps.length ? "Some history is missing" : "Full history fetched"}</span>
          </div>
        </div>
        <h3>Method</h3>
        <ul className="trace-method">
          {result.method.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </section>

      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">
              {result.edges.length} traced edges · {result.nodes.length} addresses
            </span>
            <h2>Trace graph</h2>
          </div>
        </div>
        {result.edges.length ? (
          <div className="trace-graph">
            <GraphCanvas mode="explorer" nodes={visibleGraph.nodes} edges={visibleGraph.edges} selection={selection} onSelectionChange={setSelection} />
            <SelectionInspector selection={selection} emptyMessage="Select a node or edge to inspect it." />
          </div>
        ) : (
          <p className="empty-state">No traced flows in this window.</p>
        )}
      </section>

      <EndpointTable
        title={backward ? "Sources" : "Sinks"}
        endpoints={result.sinks}
        emptyText={backward ? "No labelled sources were reached." : "No sinks were reached."}
      />
      <EndpointTable title="Stopped by limits" endpoints={result.frontier} emptyText="Nothing was cut off by the hop, branch or value limits." />

      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">{flows.length} traced transactions</span>
            <h2>Flows</h2>
          </div>
        </div>
        {flows.length ? (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Time</th>
                  <th>From → To</th>
                  <th>Type</th>
                  <th className="numeric">Traced</th>
                  <th className="numeric">USD at time</th>
                  <th>Transactions</th>
                  <th className="numeric">Confidence</th>
                </tr>
              </thead>
              <tbody>
                {flows.slice(0, FLOW_ROW_LIMIT).map(({ edge, tx }, index) => (
                  <tr key={`${edge.id}|${tx.tx_id}|${tx.asset}|${index}`}>
                    <td>{formatShortDateTime(tx.time)}</td>
                    <td>
                      {labels.get(edge.from) ?? edge.from} → {labels.get(edge.to) ?? edge.to}
                    </td>
                    <td>{edge.action_label || edge.action_class}</td>
                    <td className="numeric">
                      {tx.input_asset ? (
                        <>
                          {formatTraceAmount(tx.traced_input_amount ?? 0)} {tx.input_asset} → {formatTraceAmount(tx.traced_amount)} {tx.asset}
                        </>
                      ) : (
                        <>
                          {formatTraceAmount(tx.traced_amount)} {tx.asset}
                        </>
                      )}
                      {tx.fraction < 0.999 ? <div className="trace-fraction">{Math.round(tx.fraction * 100)}% of the payment</div> : null}
                    </td>
                    <td className="numeric">{tx.priced ? formatUSD(tx.traced_usd_at_time) : "unpriced"}</td>
                    <td>
                      {tx.inbound_tx_id ? (
                        <div>
                          in <TxLink txID={tx.inbound_tx_id} chain={chainForAsset(tx.input_asset ?? tx.asset)} />
                        </div>
                      ) : null}
                      <div>
                        {tx.inbound_tx_id ? "out " : null}
                        <TxLink txID={tx.tx_id} chain={edge.action_class === "transfers" || tx.inbound_tx_id ? chainForAsset(tx.asset) : "THOR"} />
                      </div>
                    </td>
                    <td className="numeric" title={tx.confidence_reason}>
                      {Math.round(tx.confidence * 100)}%
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="empty-state">No traced transactions.</p>
        )}
        {flows.length > FLOW_ROW_LIMIT ? <p className="form-message">Showing the first {FLOW_ROW_LIMIT} transactions.</p> : null}
      </section>

      {gaps.length || warnings.length ? (
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Coverage</span>
              <h2>Gaps and warnings</h2>
            </div>
          </div>
          {gaps.length ? (
            <>
              <h3>Coverage gaps</h3>
              <ul className="trace-method">
                {gaps.map((gap) => (
                  <li key={gap}>{gap}</li>
                ))}
              </ul>
            </>
          ) : null}
          {warnings.length ? (
            <>
              <h3>Warnings</h3>
              <ul className="trace-method">
                {warnings.map((warning) => (
                  <li key={warning}>{warning}</li>
                ))}
              </ul>
            </>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}
