import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getActorMonitor, markActorViewed, refreshActor, setActorWatch } from "../../lib/api";
import { formatShortDateTime } from "../../lib/format";
import type { Actor, ActorCounterparty } from "../../lib/types";
import { HoldingsChart } from "./HoldingsChart";

function formatUSD(value: number) {
  return new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 0 }).format(value);
}

function formatAmount(value: number) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: value >= 1000 ? 0 : 4 }).format(value);
}

function formatSignedUSD(value: number) {
  const formatted = formatUSD(Math.abs(value));
  return value > 0 ? `+${formatted}` : value < 0 ? `−${formatted}` : formatted;
}

function CategoryBadge({ category }: { category?: string }) {
  if (!category) {
    return null;
  }
  return <span className={`label-category-badge label-category-${category}`}>{category.replace(/_/g, " ")}</span>;
}

// ActorMonitorPanel shows an actor's holdings over time and what changed in
// refreshes since the user last looked.
export function ActorMonitorPanel({ actor }: { actor: Actor }) {
  const queryClient = useQueryClient();
  const queryKey = ["actor-monitor", actor.id];
  const monitorQuery = useQuery({ queryKey, queryFn: () => getActorMonitor(actor.id) });
  const [status, setStatus] = useState("");
  const [threshold, setThreshold] = useState("10000");
  const [busy, setBusy] = useState(false);
  const monitor = monitorQuery.data;
  const changes = monitor?.since_last_view;
  const minMove = Number(threshold) || 0;

  async function onRefresh() {
    setBusy(true);
    setStatus("Refreshing…");
    try {
      const snapshot = await refreshActor(actor.id, (job) => setStatus(`Refreshing — ${job.stage || "starting"}${job.message ? ` (${job.message})` : ""}…`));
      setStatus(`Refreshed ${formatShortDateTime(snapshot.taken_at)}.`);
      await queryClient.invalidateQueries({ queryKey });
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Refresh failed.");
    } finally {
      setBusy(false);
    }
  }

  async function onToggleWatch(watch: boolean) {
    await setActorWatch(actor.id, watch);
    await queryClient.invalidateQueries({ queryKey });
  }

  async function onMarkSeen() {
    await markActorViewed(actor.id);
    await queryClient.invalidateQueries({ queryKey });
  }

  const counterpartyRows = (rows: ActorCounterparty[] | null | undefined) =>
    (rows ?? []).map((cp) => {
      const total = cp.in_usd + cp.out_usd;
      return (
        <tr key={cp.key} className={total >= minMove && minMove > 0 ? "row-highlight" : undefined}>
          <td>
            <CategoryBadge category={cp.category} /> {cp.label}
            {cp.first_seen ? <span className="chip chip-accent new-badge">new</span> : null}
          </td>
          <td className="numeric">{cp.in_usd ? formatUSD(cp.in_usd) : "—"}</td>
          <td className="numeric">{cp.out_usd ? formatUSD(cp.out_usd) : "—"}</td>
          <td className="numeric">{cp.transactions}</td>
          <td>{cp.action_classes.join(", ")}</td>
        </tr>
      );
    });

  return (
    <section className="section monitor" aria-label={`${actor.name} monitoring`}>
      <div className="section-head monitor-head">
        <h2>Monitoring</h2>
        <div className="form-actions">
          <label className="check">
            <input type="checkbox" checked={Boolean(monitor?.watch)} onChange={(event) => void onToggleWatch(event.target.checked)} />
            <span>Refresh on schedule</span>
          </label>
          <button type="button" className="btn btn-sm" onClick={() => void onRefresh()} disabled={busy}>
            {busy ? "Refreshing…" : "Refresh now"}
          </button>
          <button type="button" className="btn btn-sm" onClick={() => void onMarkSeen()} disabled={!changes}>
            Mark as seen
          </button>
        </div>
      </div>
      {status ? <p className="status-text">{status}</p> : null}
      {monitorQuery.error ? <p className="error-text">{monitorQuery.error.message}</p> : null}

      {monitor?.latest ? (
        <div className="monitor-hero">
          <span className="monitor-hero-label">Holdings now</span>
          <strong className="monitor-hero-value">{formatUSD(monitor.latest.total_usd)}</strong>
          {changes ? (
            <span className="monitor-hero-delta">
              {formatSignedUSD(changes.total_usd_after - changes.total_usd_before)} since you last looked
            </span>
          ) : (
            <span className="monitor-hero-delta">No changes since you last looked</span>
          )}
        </div>
      ) : null}

      <HoldingsChart points={monitor?.series ?? []} />

      {changes ? (
        <div className="monitor-changes">
          <div className="monitor-changes-head">
            <h3>
              Since {monitor?.last_viewed_at ? formatShortDateTime(monitor.last_viewed_at) : "the first refresh"}, {changes.snapshots}{" "}
              refresh{changes.snapshots === 1 ? "" : "es"}
            </h3>
            <label className="field inline-field">
              <span>Highlight moves ≥ USD</span>
              <input type="number" min={0} step="any" value={threshold} onChange={(event) => setThreshold(event.target.value)} />
            </label>
          </div>
          <p className="section-note">
            {formatUSD(changes.flows.in_usd)} in, {formatUSD(changes.flows.out_usd)} out, across {changes.flows.transactions}{" "}
            transactions (valued at transaction time)
          </p>

          {changes.new_counterparties?.length ? (
            <>
              <h4>New counterparties</h4>
              <ul className="label-list">
                {changes.new_counterparties.map((cp) => (
                  <li key={cp.key}>
                    <CategoryBadge category={cp.category} /> {cp.label} — {formatUSD(cp.in_usd + cp.out_usd)}
                  </li>
                ))}
              </ul>
            </>
          ) : null}

          <h4>Holdings change by asset</h4>
          <div className="table-wrap trace-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Asset</th>
                  <th className="numeric">Before</th>
                  <th className="numeric">Now</th>
                  <th className="numeric">Change (USD now)</th>
                </tr>
              </thead>
              <tbody>
                {(changes.asset_deltas ?? []).map((delta) => (
                  <tr key={delta.asset} className={Math.abs(delta.delta_usd) >= minMove && minMove > 0 ? "row-highlight" : undefined}>
                    <td>{delta.asset}</td>
                    <td className="numeric">{formatAmount(delta.before_amount)}</td>
                    <td className="numeric">{formatAmount(delta.after_amount)}</td>
                    <td className="numeric">{formatSignedUSD(delta.delta_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <h4>Holdings change by address</h4>
          <div className="table-wrap trace-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Address</th>
                  <th className="numeric">Before</th>
                  <th className="numeric">Now</th>
                  <th className="numeric">Change</th>
                </tr>
              </thead>
              <tbody>
                {(changes.holding_deltas ?? []).map((delta) => (
                  <tr key={`${delta.chain}|${delta.address}`} className={Math.abs(delta.delta_usd) >= minMove && minMove > 0 ? "row-highlight" : undefined}>
                    <td>
                      {delta.label} <span className="mono-wrap">({delta.chain})</span>
                    </td>
                    <td className="numeric">{formatUSD(delta.before_usd)}</td>
                    <td className="numeric">{formatUSD(delta.after_usd)}</td>
                    <td className="numeric">{formatSignedUSD(delta.delta_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <h4>Flows by counterparty</h4>
          <div className="table-wrap trace-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Counterparty</th>
                  <th className="numeric">In</th>
                  <th className="numeric">Out</th>
                  <th className="numeric">Txs</th>
                  <th>Types</th>
                </tr>
              </thead>
              <tbody>{counterpartyRows(changes.flows.counterparties)}</tbody>
            </table>
          </div>
        </div>
      ) : monitor?.latest ? (
        <p className="empty-state">Nothing new since you last looked.</p>
      ) : null}
    </section>
  );
}
