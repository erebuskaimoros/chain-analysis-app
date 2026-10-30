import { useEffect, useState, type ReactNode } from "react";
import { AlertIcon, CloseIcon, StopIcon } from "../../../app/icons";

export interface MapProgressState {
  stage: string;
  done: number;
  total: number;
  message?: string;
  nodes?: number;
}

function progressDetail(progress: MapProgressState) {
  const parts = [];
  if (progress.total > 0) {
    parts.push(`${Math.min(progress.done, progress.total)} of ${progress.total}`);
  }
  if (progress.message) {
    parts.push(progress.message);
  }
  if (progress.nodes) {
    parts.push(`${progress.nodes} nodes so far`);
  }
  return parts.join(", ");
}

function stageText(stage: string) {
  const text = stage.replace(/_/g, " ").trim() || "starting";
  return text.charAt(0).toUpperCase() + text.slice(1);
}

// MapProgress shows a long job on the map: a card while the map is empty, a
// slim banner once a partial graph is drawn.
export function MapProgress({
  title,
  progress,
  onCancel,
  variant,
}: {
  title: string;
  progress: MapProgressState | null;
  onCancel?: () => void;
  variant: "card" | "banner";
}) {
  const stage = progress ? stageText(progress.stage) : "Starting";
  const detail = progress ? progressDetail(progress) : "";
  const percent = progress && progress.total > 0 ? Math.min(100, (progress.done / progress.total) * 100) : null;

  if (variant === "banner") {
    return (
      <div className="map-overlay map-progress-banner" role="status">
        <span className="spinner" aria-hidden="true" />
        <span>
          {title}: {stage.toLowerCase()}
          {detail ? `, ${detail}` : ""}
        </span>
        {onCancel ? (
          <button type="button" className="btn btn-sm" onClick={onCancel}>
            Cancel
          </button>
        ) : null}
      </div>
    );
  }

  return (
    <div className="map-overlay map-center">
      <div className="map-card map-progress-card" role="status" aria-live="polite">
        <h2>{title}</h2>
        <span className="map-progress-stage">{stage}</span>
        <div className={`progress${percent === null ? " is-indeterminate" : ""}`}>
          <div className="progress-bar" style={{ width: `${percent ?? 30}%` }} />
        </div>
        <span className="map-progress-detail">{detail || "Waiting for the server…"}</span>
        <p>Long scans fetch history that is not cached yet. The graph draws as soon as nodes arrive.</p>
        {onCancel ? (
          <div className="form-actions">
            <button type="button" className="btn" onClick={onCancel}>
              <StopIcon />
              Cancel
            </button>
          </div>
        ) : null}
      </div>
    </div>
  );
}

export function MapMessage({ title, children, actions }: { title: string; children?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="map-overlay map-center">
      <div className="map-empty">
        <h2>{title}</h2>
        {children ? <p>{children}</p> : null}
        {actions ? <div className="form-actions">{actions}</div> : null}
      </div>
    </div>
  );
}

// MapNotice shows the latest status message on the map for a few seconds
// while the query panel (where the status line lives) is closed.
export function MapNotice({ text }: { text: string }) {
  const [visible, setVisible] = useState(Boolean(text));

  useEffect(() => {
    if (!text) {
      setVisible(false);
      return;
    }
    setVisible(true);
    const timer = window.setTimeout(() => setVisible(false), 6000);
    return () => window.clearTimeout(timer);
  }, [text]);

  if (!text || !visible) {
    return null;
  }
  return (
    <div className="map-overlay map-notice" role="status">
      {text}
    </div>
  );
}

export function MapStatus({ children }: { children: ReactNode }) {
  return <div className="map-overlay map-status">{children}</div>;
}

interface WarningGroup {
  text: string;
  items: string[];
}

// Many provider warnings repeat with a different address at the end
// ("LTC tracker flow truncated for ltc1…"). Group those so the map stays
// readable, keeping every original line one click away.
export function groupWarnings(warnings: string[]): WarningGroup[] {
  const groups = new Map<string, string[]>();
  warnings.forEach((warning) => {
    const match = warning.match(/^(.*\S) for (\S+)$/);
    const key = match ? match[1] : warning;
    groups.set(key, [...(groups.get(key) ?? []), warning]);
  });
  return Array.from(groups.entries()).map(([key, items]) =>
    items.length > 1 ? { text: `${key} for ${items.length} addresses`, items } : { text: items[0], items: [] }
  );
}

export function MapWarnings({ warnings, onClose }: { warnings: string[]; onClose: () => void }) {
  const groups = groupWarnings(warnings);
  return (
    <div className="map-overlay map-warnings">
      <div className="map-card">
        <div className="section-head">
          <h3>
            <AlertIcon /> {warnings.length === 1 ? "1 warning" : `${warnings.length} warnings`} about this graph
          </h3>
          <button type="button" className="btn btn-sm btn-icon" aria-label="Hide warnings" onClick={onClose}>
            <CloseIcon />
          </button>
        </div>
        <ul>
          {groups.map((group) => (
            <li key={group.text}>
              {group.items.length ? (
                <details>
                  <summary>{group.text}</summary>
                  <ul>
                    {group.items.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                </details>
              ) : (
                group.text
              )}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
