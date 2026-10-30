import { useState } from "react";
import { ConfirmDialog } from "../../ui/Dialog";
import { DownloadIcon, PlayIcon, StopIcon, TrashIcon } from "../../app/icons";
import { useRouter } from "../../app/router";
import { useActiveCase } from "../../app/activeCase";
import { formatRelativeTime, pluralize } from "../../lib/format";
import type { Actor, GraphRun, GraphStateSummary } from "../../lib/types";
import { Field } from "../../ui/Field";
import { MenuButton } from "../../ui/Menu";
import { TimeWindowField } from "../../ui/TimeWindowField";
import { GraphStateLoaderButton } from "../shared/GraphStateLoaderButton";

export interface GraphFormState {
  start_time: string;
  end_time: string;
  max_hops: number;
  min_usd: string;
  include_unpriced?: boolean;
}

interface ActorGraphSidebarProps {
  actors: Actor[];
  actorsLoading?: boolean;
  selectedActorIDs: number[];
  onToggleActor: (actorID: number) => void;
  onClearActors: () => void;
  form: GraphFormState;
  onFormChange: (next: GraphFormState) => void;
  onBuild: () => void;
  onCancelBuild: () => void;
  isBuilding: boolean;
  canBuild: boolean;
  statusText: string;
  runs: GraphRun[];
  onRunAgain: (run: GraphRun) => void;
  onDeleteRun: (run: GraphRun) => void;
  isLoadingRuns: boolean;
  savedStates: GraphStateSummary[];
  onOpenSavedState: (state: GraphStateSummary) => void;
  onDeleteSavedState: (state: GraphStateSummary) => void;
  onExportSavedState: (state: GraphStateSummary) => void;
  onLoadGraphFile: (file: File) => void;
  isLoadingSavedStates: boolean;
}

const RECENT_LIMIT = 5;

export function ActorGraphSidebar({
  actors,
  actorsLoading = false,
  selectedActorIDs,
  onToggleActor,
  onClearActors,
  form,
  onFormChange,
  onBuild,
  onCancelBuild,
  isBuilding,
  canBuild,
  statusText,
  runs,
  onRunAgain,
  onDeleteRun,
  isLoadingRuns,
  savedStates,
  onOpenSavedState,
  onDeleteSavedState,
  onExportSavedState,
  onLoadGraphFile,
  isLoadingSavedStates,
}: ActorGraphSidebarProps) {
  const { navigate } = useRouter();
  const { addToCase } = useActiveCase();
  const [showAllRuns, setShowAllRuns] = useState(false);
  const [confirmDeleteState, setConfirmDeleteState] = useState<GraphStateSummary | null>(null);
  const visibleRuns = showAllRuns ? runs : runs.slice(0, RECENT_LIMIT);

  return (
    <>
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          onBuild();
        }}
      >
        <div className="ws-group">
          <div className="ws-group-head">
            <h3>Actors</h3>
            {selectedActorIDs.length ? (
              <button type="button" className="link-btn" onClick={onClearActors}>
                Clear {selectedActorIDs.length}
              </button>
            ) : null}
          </div>
          {actors.length ? (
            <div className="chip-set" role="group" aria-label="Actors to graph">
              {actors.map((actor) => (
                <label key={actor.id} className="toggle-chip">
                  <input
                    type="checkbox"
                    className="visually-hidden"
                    checked={selectedActorIDs.includes(actor.id)}
                    onChange={() => onToggleActor(actor.id)}
                  />
                  <span className="swatch" style={{ background: actor.color || "#4ca3ff" }} />
                  <span>{actor.name}</span>
                  <span className="count" aria-label={pluralize(actor.addresses.length, "address", "addresses")}>
                    {actor.addresses.length}
                  </span>
                </label>
              ))}
            </div>
          ) : actorsLoading ? (
            <p className="status-text">Loading actors…</p>
          ) : (
            <div className="empty-state">
              An actor is a named set of addresses. Create one first, then graph its flows here.
              <button type="button" className="btn btn-sm" onClick={() => navigate("actors", { new: "1" })}>
                Create an actor
              </button>
            </div>
          )}
        </div>

        <TimeWindowField
          start={form.start_time}
          end={form.end_time}
          onChange={(next) => onFormChange({ ...form, start_time: next.start, end_time: next.end })}
        />

        <Field label="Max hops" hint="How far to follow flows out from the actors' own addresses.">
          <input
            type="number"
            min={1}
            max={8}
            value={form.max_hops}
            onChange={(event) => onFormChange({ ...form, max_hops: Number(event.target.value) || form.max_hops })}
          />
        </Field>

        <details className="disclosure">
          <summary>More options</summary>
          <div className="disclosure-body">
            <Field label="Min USD (at time)" hint="Leave out flows worth less than this when they happened.">
              <input
                type="number"
                step="any"
                value={form.min_usd}
                onChange={(event) => onFormChange({ ...form, min_usd: event.target.value })}
              />
            </Field>
            <label className="check" title="Keep flows of assets with no known price when a minimum is set">
              <input
                type="checkbox"
                checked={Boolean(form.include_unpriced)}
                onChange={(event) => onFormChange({ ...form, include_unpriced: event.target.checked })}
              />
              <span>Include unpriced assets</span>
            </label>
          </div>
        </details>

        <div className="form-actions">
          {isBuilding ? (
            <button type="button" className="btn" onClick={onCancelBuild}>
              <StopIcon />
              Cancel build
            </button>
          ) : (
            <button type="submit" className="btn btn-primary" disabled={!canBuild}>
              Build graph
            </button>
          )}
        </div>
        <p className="status-text" role="status">
          {statusText}
        </p>
      </form>

      <section className="ws-group" aria-label="Saved graphs">
        <div className="ws-group-head">
          <h3>Saved graphs</h3>
          <GraphStateLoaderButton
            className="btn btn-ghost btn-sm"
            label="Open a file…"
            disabled={isBuilding}
            onLoadFile={(file) => onLoadGraphFile(file)}
          />
        </div>
        {isLoadingSavedStates ? <p className="status-text">Loading saved graphs…</p> : null}
        {savedStates.length ? (
          <ul className="list ws-run-list">
            {savedStates.map((state) => (
              <li key={state.id} className="list-row">
                <button
                  type="button"
                  className="link-btn list-row-main"
                  title={`Open ${state.name} exactly as it was saved`}
                  onClick={() => onOpenSavedState(state)}
                >
                  <span className="list-row-title">{state.name}</span>
                  <span className="list-row-meta">
                    {pluralize(state.node_count, "node")}, saved {formatRelativeTime(state.updated_at)}
                  </span>
                </button>
                <MenuButton
                  label={`Actions for ${state.name}`}
                  items={[
                    { label: "Open", onSelect: () => onOpenSavedState(state) },
                    {
                      label: "Add to the active case",
                      onSelect: () => addToCase([{ kind: "graph_state", ref: String(state.id) }], state.name),
                    },
                    { label: "Export as a file", icon: <DownloadIcon />, onSelect: () => onExportSavedState(state) },
                    "separator",
                    { label: "Delete…", icon: <TrashIcon />, danger: true, onSelect: () => setConfirmDeleteState(state) },
                  ]}
                />
              </li>
            ))}
          </ul>
        ) : !isLoadingSavedStates ? (
          <p className="section-note">
            Save a graph from the map toolbar to reopen it here exactly as you left it, layout and filters included.
          </p>
        ) : null}
      </section>

      <section className="ws-group" aria-label="Recent builds">
        <div className="ws-group-head">
          <h3>Recent builds</h3>
          {runs.length > RECENT_LIMIT ? (
            <button type="button" className="link-btn" onClick={() => setShowAllRuns((value) => !value)}>
              {showAllRuns ? "Show fewer" : `Show all ${runs.length}`}
            </button>
          ) : null}
        </div>
        {isLoadingRuns ? <p className="status-text">Loading recent builds…</p> : null}
        {runs.length ? (
          <ul className="list ws-run-list">
            {visibleRuns.map((run) => (
              <li key={run.id} className="list-row">
                <span className="list-row-main">
                  <span className="list-row-title">{run.actor_names || "Untitled"}</span>
                  <span className="list-row-meta">
                    {pluralize(run.node_count, "node")}, {formatRelativeTime(run.created_at)}
                  </span>
                </span>
                <button
                  type="button"
                  className="btn btn-sm"
                  disabled={isBuilding}
                  title="Builds the graph again with this run's actors and settings"
                  onClick={() => onRunAgain(run)}
                >
                  <PlayIcon size={12} />
                  Run again
                </button>
                <MenuButton
                  label={`Actions for the ${run.actor_names || "untitled"} run`}
                  items={[{ label: "Delete", icon: <TrashIcon />, danger: true, onSelect: () => onDeleteRun(run) }]}
                />
              </li>
            ))}
          </ul>
        ) : !isLoadingRuns ? (
          <p className="section-note">Each graph you build is listed here so you can run it again.</p>
        ) : null}
      </section>

      {confirmDeleteState ? (
        <ConfirmDialog
          title={`Delete ${confirmDeleteState.name}?`}
          message="The saved layout, filters and expansions go with it. Export it as a file first if you might want it back."
          confirmLabel="Delete saved graph"
          onCancel={() => setConfirmDeleteState(null)}
          onConfirm={() => {
            const target = confirmDeleteState;
            setConfirmDeleteState(null);
            onDeleteSavedState(target);
          }}
        />
      ) : null}
    </>
  );
}
