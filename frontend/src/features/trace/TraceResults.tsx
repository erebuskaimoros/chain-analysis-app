import { useMemo, useState } from "react";
import { PinIcon } from "../../app/icons";
import { useActiveCase } from "../../app/activeCase";
import { useRouter } from "../../app/router";
import { formatShortDateTime, formatUSD, middleTruncate, pluralize, shortHash } from "../../lib/format";
import { chainForAsset, explorerURLForAddress, explorerURLForTx } from "../../lib/graph/actions";
import { deriveExplorerVisibleGraph } from "../../lib/graph/derive";
import { createGraphFilterState } from "../../lib/graph/filters";
import type { GraphSelection } from "../../lib/graph/types";
import { encodeSeed } from "../../lib/identify";
import type { AddressExplorerResponse, TraceAmount, TraceEndpoint, TraceResponse } from "../../lib/types";
import { ChainBadge } from "../../ui/AddressText";
import { MenuButton, type MenuItem } from "../../ui/Menu";
import { GraphCanvas } from "../shared/GraphCanvas";
import { SelectionInspector } from "../shared/SelectionInspector";
import { TraceOutcomeChart } from "./TraceOutcomeChart";

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
    return <span className="mono">{shortHash(txID)}</span>;
  }
  return (
    <a className="mono" href={url} target="_blank" rel="noreferrer" title={txID}>
      {shortHash(txID)}
    </a>
  );
}

function EndpointTable({
  title,
  endpoints,
  emptyText,
  actionsFor,
}: {
  title: string;
  endpoints: TraceEndpoint[];
  emptyText: string;
  actionsFor: (endpoint: TraceEndpoint) => MenuItem[];
}) {
  return (
    <section className="section" aria-label={title}>
      <div className="section-head">
        <h2>
          {title} <span className="count">{endpoints.length}</span>
        </h2>
      </div>
      {endpoints.length ? (
        <div className="table-wrap trace-table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Why it stops</th>
                <th className="numeric">Traced</th>
                <th className="numeric">Value at the time</th>
                <th className="numeric">Confidence</th>
                <th className="numeric">Holds now</th>
                <th className="numeric">Hop</th>
                <th className="cell-actions" aria-label="Row actions" />
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
                        <a href={url} target="_blank" rel="noreferrer" title={endpoint.address}>
                          {endpoint.label}
                        </a>
                      ) : (
                        endpoint.label
                      )}
                      {endpoint.chain ? (
                        <>
                          {" "}
                          <ChainBadge chain={endpoint.chain} />
                        </>
                      ) : null}
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
                    <td className="cell-actions">
                      {endpoint.address ? (
                        <MenuButton label={`Actions for ${endpoint.label}`} items={actionsFor(endpoint)} />
                      ) : null}
                    </td>
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

function summarySentence(result: TraceResponse) {
  const backward = result.query.direction === "backward";
  const seeds = pluralize(result.query.seeds.length, "seed");
  const reached = result.sinks.length;
  return (
    <>
      <strong className="num">{formatUSD(result.totals.seed_usd)}</strong> traced {backward ? "back into" : "forward from"} {seeds}{" "}
      reached {pluralize(reached, backward ? "labelled source" : "endpoint")} worth{" "}
      <strong className="num">{formatUSD(result.totals.sink_usd)}</strong>
      {result.totals.frontier_usd > 0 ? (
        <>
          , and <strong className="num">{formatUSD(result.totals.frontier_usd)}</strong> stopped at the hop, branch or value limits
        </>
      ) : null}
      .
    </>
  );
}

export function TraceResults({ result }: { result: TraceResponse }) {
  const [selection, setSelection] = useState<GraphSelection>(null);
  const { addToCase } = useActiveCase();
  const { navigate } = useRouter();
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

  function endpointActions(endpoint: TraceEndpoint): MenuItem[] {
    const address = endpoint.address ?? "";
    const seed = encodeSeed(address, endpoint.chain);
    const url = explorerURLForAddress(address, endpoint.chain ?? "");
    const items: MenuItem[] = [
      { label: "Explore this address", onSelect: () => navigate("explorer", { address }) },
      { label: "Trace on from here", onSelect: () => navigate("trace", { seed }) },
      {
        label: "Add to the active case",
        onSelect: () => addToCase([{ kind: "address", ref: seed }], endpoint.label || middleTruncate(address)),
      },
      { label: "Copy address", onSelect: () => void navigator.clipboard?.writeText(address) },
    ];
    if (url) {
      items.push({ label: "Open in a block explorer", onSelect: () => window.open(url, "_blank", "noopener,noreferrer") });
    }
    return items;
  }

  return (
    <div className="trace-results">
      <div className="trace-summary">
        <p className="trace-summary-text">{summarySentence(result)}</p>
        {result.run_id ? (
          <button
            type="button"
            className="btn"
            onClick={() => addToCase([{ kind: "trace_run", ref: String(result.run_id) }], `trace ${result.run_id}`)}
          >
            <PinIcon />
            Add trace to case
          </button>
        ) : null}
      </div>

      <TraceOutcomeChart result={result} />

      <section className="section" aria-label="Trace graph">
        <div className="section-head">
          <h2>
            Trace graph <span className="count">{pluralize(result.edges.length, "flow")}</span>
          </h2>
          <span className="section-note">{pluralize(result.nodes.length, "address", "addresses")}</span>
        </div>
        {result.edges.length ? (
          <div className="trace-graph graph-embed">
            <GraphCanvas mode="explorer" nodes={visibleGraph.nodes} edges={visibleGraph.edges} selection={selection} onSelectionChange={setSelection} />
            <div className="section">
              <SelectionInspector
                selection={selection}
                emptyMessage="Click an address or a flow on the graph to see its details."
                nodeLabel={(id) => labels.get(id) ?? id}
              />
            </div>
          </div>
        ) : (
          <p className="empty-state">No traced flows in this window.</p>
        )}
      </section>

      <EndpointTable
        title={backward ? "Sources" : "Sinks"}
        endpoints={result.sinks}
        emptyText={backward ? "No labelled sources were reached." : "No sinks were reached."}
        actionsFor={endpointActions}
      />
      <EndpointTable
        title="Stopped by limits"
        endpoints={result.frontier}
        emptyText="Nothing was cut off by the hop, branch or value limits."
        actionsFor={endpointActions}
      />

      <section className="section" aria-label="Flows">
        <div className="section-head">
          <h2>
            Flows <span className="count">{flows.length}</span>
          </h2>
          <span className="section-note">Every traced payment, oldest first</span>
        </div>
        {flows.length ? (
          <div className="table-wrap trace-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Time</th>
                  <th>From → To</th>
                  <th>Type</th>
                  <th className="numeric">Traced</th>
                  <th className="numeric">Value at the time</th>
                  <th>Transactions</th>
                  <th className="numeric">Confidence</th>
                </tr>
              </thead>
              <tbody>
                {flows.slice(0, FLOW_ROW_LIMIT).map(({ edge, tx }, index) => (
                  <tr key={`${edge.id}|${tx.tx_id}|${tx.asset}|${index}`}>
                    <td className="cell-muted">{formatShortDateTime(tx.time)}</td>
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
        {flows.length > FLOW_ROW_LIMIT ? <p className="section-note">Showing the first {FLOW_ROW_LIMIT} transactions.</p> : null}
      </section>

      <div className="trace-two">
        <section className="section" aria-label="How this was traced">
          <h2>How this was traced</h2>
          <ul className="trace-method">
            {result.method.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </section>
        <section className="section" aria-label="Coverage">
          <h2>Coverage</h2>
          {gaps.length || warnings.length ? (
            <>
              {gaps.length ? (
                <>
                  <h3 className="section-title">Coverage gaps</h3>
                  <ul className="trace-method">
                    {gaps.map((gap) => (
                      <li key={gap}>{gap}</li>
                    ))}
                  </ul>
                </>
              ) : null}
              {warnings.length ? (
                <>
                  <h3 className="section-title">Warnings</h3>
                  <ul className="trace-method">
                    {warnings.map((warning) => (
                      <li key={warning}>{warning}</li>
                    ))}
                  </ul>
                </>
              ) : null}
            </>
          ) : (
            <p className="section-note">Full history was fetched for every address in the trace.</p>
          )}
        </section>
      </div>
    </div>
  );
}
