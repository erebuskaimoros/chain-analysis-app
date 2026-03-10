import { formatShortDateTime } from "../../lib/format";
import type { Actor, GraphRun } from "../../lib/types";

export interface GraphFormState {
  start_time: string;
  end_time: string;
  max_hops: number;
  min_usd: string;
}

interface ActorGraphSidebarProps {
  actors: Actor[];
  selectedActorIDs: number[];
  onToggleActor: (actorID: number) => void;
  form: GraphFormState;
  onFormChange: (next: GraphFormState) => void;
  onBuild: () => void;
  isBuilding: boolean;
  canBuild: boolean;
  onRefreshLiveHoldings: () => void;
  canRefreshLiveHoldings: boolean;
  isRefreshingLiveHoldings: boolean;
  statusText: string;
  runs: GraphRun[];
  selectedRunID: string;
  onSelectedRunIDChange: (value: string) => void;
  onLoadRun: () => void;
  onDeleteRun: () => void;
  isDeletingRun: boolean;
  hasSelectedRun: boolean;
  isLoadingRuns: boolean;
}

export function ActorGraphSidebar({
  actors,
  selectedActorIDs,
  onToggleActor,
  form,
  onFormChange,
  onBuild,
  isBuilding,
  canBuild,
  onRefreshLiveHoldings,
  canRefreshLiveHoldings,
  isRefreshingLiveHoldings,
  statusText,
  runs,
  selectedRunID,
  onSelectedRunIDChange,
  onLoadRun,
  onDeleteRun,
  isDeletingRun,
  hasSelectedRun,
  isLoadingRuns,
}: ActorGraphSidebarProps) {
  return (
    <div className="page-stack">
      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Actor Graph</span>
            <h2>Build Graph</h2>
          </div>
        </div>
        <form
          className="form-grid"
          onSubmit={(event) => {
            event.preventDefault();
            onBuild();
          }}
        >
          <div className="field field-full">
            <span>Actors</span>
            <div className="checkbox-list">
              {actors.map((actor) => (
                <label key={actor.id} className="checkbox-card">
                  <input
                    type="checkbox"
                    checked={selectedActorIDs.includes(actor.id)}
                    onChange={() => onToggleActor(actor.id)}
                  />
                  <span className="actor-color-swatch" style={{ background: actor.color || "#4ca3ff" }} />
                  <span>{actor.name}</span>
                  <span className="badge">{actor.addresses.length}</span>
                </label>
              ))}
            </div>
          </div>
          <label className="field">
            <span>Start</span>
            <input
              type="datetime-local"
              value={form.start_time}
              onChange={(event) => onFormChange({ ...form, start_time: event.target.value })}
            />
          </label>
          <label className="field">
            <span>End</span>
            <input
              type="datetime-local"
              value={form.end_time}
              onChange={(event) => onFormChange({ ...form, end_time: event.target.value })}
            />
          </label>
          <label className="field">
            <span>Max Hops</span>
            <input
              type="number"
              min={1}
              max={8}
              value={form.max_hops}
              onChange={(event) => onFormChange({ ...form, max_hops: Number(event.target.value) || form.max_hops })}
            />
          </label>
          <label className="field">
            <span>Min USD</span>
            <input
              type="number"
              step="any"
              value={form.min_usd}
              onChange={(event) => onFormChange({ ...form, min_usd: event.target.value })}
            />
          </label>
          <div className="form-actions field-full">
            <button type="submit" className="button" disabled={isBuilding || !canBuild}>
              {isBuilding ? "Building..." : "Build Graph"}
            </button>
            <button
              type="button"
              className="button secondary"
              disabled={!canRefreshLiveHoldings || isRefreshingLiveHoldings}
              onClick={onRefreshLiveHoldings}
            >
              {isRefreshingLiveHoldings ? "Refreshing..." : "Refresh Live Holdings"}
            </button>
          </div>
        </form>
        <p className="status-line">{statusText}</p>
      </section>

      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Runs</span>
            <h2>Saved Graph Runs</h2>
          </div>
          <span className="status-pill ok">{runs.length}</span>
        </div>
        {isLoadingRuns ? <div className="empty-state">Loading runs…</div> : null}
        <div className="stack">
          <select value={selectedRunID} onChange={(event) => onSelectedRunIDChange(event.target.value)}>
            <option value="">Select a saved run</option>
            {runs.map((run) => (
              <option key={run.id} value={run.id}>
                {run.actor_names || "Untitled"} · {run.node_count}N/{run.edge_count}E · {formatShortDateTime(run.created_at)}
              </option>
            ))}
          </select>
          <div className="button-row">
            <button type="button" className="button secondary" disabled={!hasSelectedRun} onClick={onLoadRun}>
              Load
            </button>
            <button
              type="button"
              className="button secondary danger"
              disabled={!hasSelectedRun || isDeletingRun}
              onClick={onDeleteRun}
            >
              Delete
            </button>
          </div>
        </div>
      </section>
    </div>
  );
}
