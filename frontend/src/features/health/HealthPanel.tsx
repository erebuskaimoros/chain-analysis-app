import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getHealth } from "../../lib/api";
import { formatShortDateTime } from "../../lib/format";

function sourceCount(value: unknown): number {
  if (Array.isArray(value)) {
    return value.length;
  }
  if (value && typeof value === "object") {
    return Object.keys(value as Record<string, unknown>).length;
  }
  return 0;
}

function sourceList(value: string[] | null | undefined): string[] {
  return Array.isArray(value) ? value : [];
}

export function useHealth() {
  return useQuery({
    queryKey: ["health"],
    queryFn: getHealth,
    refetchInterval: 60_000,
  });
}

// HealthPanel lists what the local server is connected to: its build, the
// THORChain and MAYA data sources, and the external-chain trackers.
export function HealthPanel() {
  const { data, isLoading, error } = useHealth();

  if (isLoading) {
    return <p className="status-text">Checking the server…</p>;
  }

  if (error) {
    return (
      <p className="error-text">
        The server did not answer: {error instanceof Error ? error.message : "unknown error"}. Start it with make restart-server.
      </p>
    );
  }

  if (!data) {
    return null;
  }

  const trackerStates = Object.keys(data.tracker_health || {}).length;
  const trackerSources = sourceCount(data.tracker_sources);
  const engines = Object.values(data.liquidity_engines || {});

  return (
    <dl className="detail-list">
      <dt>Status</dt>
      <dd>{data.ok ? "Healthy" : "Degraded"}</dd>
      <dt>Build</dt>
      <dd>
        <span className="mono">{data.build.commit}</span>, {formatShortDateTime(data.build.build_time)}
      </dd>
      <dt>Liquidity engines</dt>
      <dd>{engines.map((engine) => engine.protocol).join(", ") || "None configured"}</dd>
      {engines.map((engine) => {
        const nodeLabel = engine.protocol === "MAYA" ? "MAYANode" : "THORNode";
        const thornodeSources = sourceList(engine.thornode_sources);
        const midgardSources = sourceList(engine.midgard_sources);
        const legacySources = sourceList(engine.legacy_action_sources);
        const summary = [
          `${nodeLabel} ${thornodeSources.length}`,
          `Midgard ${midgardSources.length}`,
          legacySources.length ? `Legacy ${legacySources.length}` : "",
        ]
          .filter(Boolean)
          .join(", ");
        return (
          <div key={engine.protocol} style={{ display: "contents" }}>
            <dt>{engine.protocol}</dt>
            <dd>{summary || "No sources configured"}</dd>
          </div>
        );
      })}
      <dt>Trackers</dt>
      <dd>
        {trackerStates} tracked, {trackerSources} source groups
      </dd>
    </dl>
  );
}

// HealthIndicator is the status dot at the foot of the navigation rail.
export function HealthIndicator() {
  const { data, error, isLoading } = useHealth();
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLDivElement | null>(null);
  const state = error ? "bad" : data ? (data.ok ? "ok" : "bad") : "unknown";
  const label = isLoading ? "Connecting…" : error ? "Server unreachable" : data?.ok ? "Server healthy" : "Server degraded";

  useEffect(() => {
    if (!open) {
      return;
    }
    function onPointerDown(event: MouseEvent) {
      if (anchorRef.current && event.target instanceof Node && !anchorRef.current.contains(event.target)) {
        setOpen(false);
      }
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  return (
    <div className="menu-anchor" ref={anchorRef} style={{ flex: 1, minWidth: 0 }}>
      <button
        type="button"
        className="btn btn-ghost btn-sm health-button"
        aria-expanded={open}
        title={label}
        onClick={() => setOpen((current) => !current)}
      >
        <span className={`health-dot is-${state}`} />
        <span className="health-label">{label}</span>
      </button>
      {open ? (
        <div className="popover health-popover" role="dialog" aria-label="Server health">
          <h3>Server</h3>
          <HealthPanel />
        </div>
      ) : null}
    </div>
  );
}
