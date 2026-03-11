import { useQuery } from "@tanstack/react-query";
import { getHealth } from "../../lib/api";

function sourceCount(value: unknown): number {
  if (Array.isArray(value)) {
    return value.length;
  }
  if (value && typeof value === "object") {
    return Object.keys(value as Record<string, unknown>).length;
  }
  return 0;
}

export function HealthPanel() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["health"],
    queryFn: getHealth,
  });

  if (isLoading) {
    return <section className="panel shell-panel">Loading health snapshot…</section>;
  }

  if (error) {
    return (
      <section className="panel shell-panel">
        <div className="panel-head">
          <h2>Health</h2>
        </div>
        <p className="error-text">{error instanceof Error ? error.message : "Unknown error"}</p>
      </section>
    );
  }

  if (!data) {
    return null;
  }

  const trackerStates = Object.keys(data.tracker_health || {}).length;
  const trackerSources = sourceCount(data.tracker_sources);
  const engines = Object.values(data.liquidity_engines || {});

  return (
    <section className="panel shell-panel">
      <div className="panel-head">
        <div>
          <span className="eyebrow">System</span>
          <h2>Health Snapshot</h2>
        </div>
        <span className={`status-pill ${data.ok ? "ok" : "bad"}`}>{data.ok ? "Healthy" : "Degraded"}</span>
      </div>
      <div className="metric-grid">
        <article className="metric-card">
          <span className="metric-label">Build</span>
          <strong>{data.build.commit}</strong>
          <small>{data.build.build_time}</small>
        </article>
        <article className="metric-card">
          <span className="metric-label">Liquidity Engines</span>
          <strong>{engines.length}</strong>
          <small>{engines.map((engine) => engine.protocol).join(" • ") || "None configured"}</small>
        </article>
        <article className="metric-card">
          <span className="metric-label">Tracker State</span>
          <strong>{trackerStates}</strong>
          <small>{trackerSources} configured source groups</small>
        </article>
        {engines.map((engine) => {
          const nodeLabel = engine.protocol === "MAYA" ? "MAYANode" : "THORNode";
          const summary = [
            `${nodeLabel} ${engine.thornode_sources.length}`,
            `Midgard ${engine.midgard_sources.length}`,
            engine.legacy_action_sources.length ? `Legacy ${engine.legacy_action_sources.length}` : "",
          ]
            .filter(Boolean)
            .join(" • ");
          return (
            <article key={engine.protocol} className="metric-card">
              <span className="metric-label">{engine.protocol}</span>
              <strong>{engine.thornode_sources.length + engine.midgard_sources.length + engine.legacy_action_sources.length}</strong>
              <small>{summary || "No sources configured"}</small>
            </article>
          );
        })}
      </div>
    </section>
  );
}
