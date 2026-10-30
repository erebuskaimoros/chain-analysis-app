import { useMemo } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { ExplorerIcon, GraphIcon, PlusIcon, TraceIcon } from "../../app/icons";
import { useActiveCase } from "../../app/activeCase";
import { useRouter, type ViewKey } from "../../app/router";
import { Omnibox } from "../../app/search";
import {
  getActorMonitor,
  listActorGraphRuns,
  listActors,
  listAddressExplorerRuns,
  listAnnotations,
  listGraphStates,
  listTraceRuns,
} from "../../lib/api";
import { formatCompactUSD, formatRelativeTime, middleTruncate, pluralize } from "../../lib/format";
import { PageHeader } from "../../ui/PageHeader";

interface ActivityEntry {
  key: string;
  at: string;
  icon: JSX.Element;
  title: string;
  meta: string;
  actionLabel: string;
  view: ViewKey;
  params: Record<string, string>;
}

const ACTIVITY_LIMIT = 12;

export function HomePage() {
  const { navigate } = useRouter();
  const { cases, activeCase, setActiveCaseID } = useActiveCase();
  const graphRunsQuery = useQuery({ queryKey: ["actor-graph-runs"], queryFn: listActorGraphRuns });
  const graphStatesQuery = useQuery({ queryKey: ["graph-states", "actor-graph"], queryFn: () => listGraphStates("actor-graph") });
  const traceRunsQuery = useQuery({ queryKey: ["trace-runs"], queryFn: listTraceRuns });
  const explorerRunsQuery = useQuery({ queryKey: ["address-explorer-runs"], queryFn: listAddressExplorerRuns });
  const actorsQuery = useQuery({ queryKey: ["actors"], queryFn: listActors });
  const annotationsQuery = useQuery({ queryKey: ["annotations"], queryFn: listAnnotations });
  const actors = useMemo(() => actorsQuery.data ?? [], [actorsQuery.data]);
  const monitorQueries = useQueries({
    queries: actors.map((actor) => ({
      queryKey: ["actor-monitor", actor.id],
      queryFn: () => getActorMonitor(actor.id),
    })),
  });

  const labelByAddress = useMemo(
    () =>
      new Map(
        (annotationsQuery.data ?? [])
          .filter((annotation) => annotation.kind === "label")
          .map((annotation) => [annotation.address.toLowerCase(), annotation.value])
      ),
    [annotationsQuery.data]
  );

  const activity = useMemo(() => {
    const entries: ActivityEntry[] = [];
    (graphStatesQuery.data ?? []).forEach((state) =>
      entries.push({
        key: `state-${state.id}`,
        at: state.updated_at,
        icon: <GraphIcon />,
        title: state.name,
        meta: `Saved graph, ${pluralize(state.node_count, "node")}`,
        actionLabel: "Open",
        view: "graph",
        params: { state: String(state.id) },
      })
    );
    (graphRunsQuery.data ?? []).forEach((run) =>
      entries.push({
        key: `run-${run.id}`,
        at: run.created_at,
        icon: <GraphIcon />,
        title: run.actor_names || "Untitled graph",
        meta: `Graph build, ${pluralize(run.node_count, "node")}`,
        actionLabel: "Run again",
        view: "graph",
        params: { run_id: String(run.id) },
      })
    );
    (traceRunsQuery.data ?? []).forEach((run) =>
      entries.push({
        key: `trace-${run.id}`,
        at: run.created_at,
        icon: <TraceIcon />,
        title: run.title,
        meta: `${run.direction === "backward" ? "Backward" : "Forward"} trace, ${formatCompactUSD(run.summary.seed_usd)} traced to ${pluralize(run.summary.sinks, "endpoint")}`,
        actionLabel: "Open",
        view: "trace",
        params: { trace: String(run.id) },
      })
    );
    (explorerRunsQuery.data ?? []).forEach((run) =>
      entries.push({
        key: `explore-${run.id}`,
        at: run.created_at,
        icon: <ExplorerIcon />,
        title: labelByAddress.get(run.request.address.toLowerCase()) || middleTruncate(run.request.address, 10, 6),
        meta: `Exploration, ${pluralize(run.node_count, "node")}`,
        actionLabel: "Open",
        view: "explorer",
        params: { address: run.request.address },
      })
    );
    // One line per thing you worked on: repeated builds of the same actors
    // (or explorations of the same address) collapse to the latest.
    const seen = new Set<string>();
    return entries
      .sort((left, right) => new Date(right.at).getTime() - new Date(left.at).getTime())
      .filter((entry) => {
        const key = `${entry.view}:${entry.view === "graph" && entry.params.run_id ? `run:${entry.title}` : entry.title}`;
        if (seen.has(key)) {
          return false;
        }
        seen.add(key);
        return true;
      })
      .slice(0, ACTIVITY_LIMIT);
  }, [explorerRunsQuery.data, graphRunsQuery.data, graphStatesQuery.data, labelByAddress, traceRunsQuery.data]);

  const watched = actors
    .map((actor, index) => ({ actor, monitor: monitorQueries[index]?.data }))
    .filter((entry) => entry.monitor?.watch);

  const activityLoading =
    graphRunsQuery.isLoading || traceRunsQuery.isLoading || explorerRunsQuery.isLoading || graphStatesQuery.isLoading;

  return (
    <>
      <PageHeader title="Home" />
      <div className="page-body home">
        <section className="home-search" aria-label="Start an investigation">
          <Omnibox inline autoFocus placeholder="Paste an address or transaction hash, or type an actor, label or case" />
          <p className="section-note">
            Search works from every page with <kbd>⌘</kbd>
            <kbd>K</kbd>.
          </p>
        </section>

        <div className="home-grid">
          <section className="section home-activity" aria-labelledby="home-activity-title">
            <div className="section-head">
              <h2 id="home-activity-title">Pick up where you left off</h2>
            </div>
            {activity.length ? (
              <ul className="list ws-run-list">
                {activity.map((entry) => (
                  <li key={entry.key} className="list-row">
                    <span className="palette-item-icon">{entry.icon}</span>
                    <span className="list-row-main">
                      <span className="list-row-title">{entry.title}</span>
                      <span className="list-row-meta">{entry.meta}</span>
                    </span>
                    <span className="list-row-side">{formatRelativeTime(entry.at)}</span>
                    <button type="button" className="btn btn-sm" onClick={() => navigate(entry.view, entry.params)}>
                      {entry.actionLabel}
                    </button>
                  </li>
                ))}
              </ul>
            ) : activityLoading ? (
              <p className="status-text">Loading recent work…</p>
            ) : (
              <div className="empty-state">
                <strong>Nothing here yet</strong>
                Graphs you build, addresses you explore and funds you trace show up here so you can reopen them.
                <div className="form-actions">
                  <button type="button" className="btn btn-sm" onClick={() => navigate("graph")}>
                    Build a graph
                  </button>
                  <button type="button" className="btn btn-sm" onClick={() => navigate("trace")}>
                    Trace funds
                  </button>
                </div>
              </div>
            )}
          </section>

          <div className="home-side">
            <section className="section" aria-labelledby="home-cases-title">
              <div className="section-head">
                <h2 id="home-cases-title">Cases</h2>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => navigate("cases", { new: "1" })}>
                  <PlusIcon />
                  New case
                </button>
              </div>
              {cases.length ? (
                <ul className="list ws-run-list">
                  {cases.slice(0, 6).map((item) => (
                    <li key={item.id} className={`list-row${activeCase?.id === item.id ? " is-selected" : ""}`}>
                      <button
                        type="button"
                        className="link-btn list-row-main"
                        onClick={() => navigate("cases", { case: String(item.id) })}
                      >
                        <span className="list-row-title">{item.title}</span>
                        <span className="list-row-meta">
                          {pluralize(item.item_count, "item")}, updated {formatRelativeTime(item.updated_at)}
                        </span>
                      </button>
                      {activeCase?.id === item.id ? (
                        <span className="chip chip-accent">Active</span>
                      ) : (
                        <button type="button" className="btn btn-ghost btn-sm" onClick={() => setActiveCaseID(item.id)}>
                          Work on this
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="section-note">
                  A case collects the addresses, transactions, traces and graphs behind a finding, and exports them as a report.
                </p>
              )}
            </section>

            <section className="section" aria-labelledby="home-watched-title">
              <div className="section-head">
                <h2 id="home-watched-title">Watched actors</h2>
              </div>
              {watched.length ? (
                <ul className="list ws-run-list">
                  {watched.map(({ actor, monitor }) => {
                    const changes = monitor?.since_last_view;
                    const delta = changes ? changes.total_usd_after - changes.total_usd_before : 0;
                    return (
                      <li key={actor.id} className="list-row">
                        <span className="swatch" style={{ background: actor.color || "#4ca3ff" }} />
                        <button
                          type="button"
                          className="link-btn list-row-main"
                          onClick={() => navigate("actors", { actor: String(actor.id) })}
                        >
                          <span className="list-row-title">{actor.name}</span>
                          <span className="list-row-meta">
                            {monitor?.latest ? `Holds ${formatCompactUSD(monitor.latest.total_usd)}` : "Not refreshed yet"}
                          </span>
                        </button>
                        {changes ? (
                          <span className="chip num">
                            {delta >= 0 ? "+" : "−"}
                            {formatCompactUSD(Math.abs(delta))} since you looked
                          </span>
                        ) : (
                          <span className="list-row-side">No change</span>
                        )}
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <p className="section-note">
                  Turn on scheduled refreshes for an actor on the Actors page to see its holdings and new flows here.
                </p>
              )}
            </section>
          </div>
        </div>
      </div>
    </>
  );
}
