import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listAnnotations } from "../../lib/api";
import { formatShortDateTime, formatUSD, shortHash } from "../../lib/format";
import { GraphFilterPopover } from "../shared/GraphFilterPopover";
import { GraphCanvas } from "../shared/GraphCanvas";
import { GraphStateLoaderButton } from "../shared/GraphStateLoaderButton";
import { SelectionInspector } from "../shared/SelectionInspector";
import { ActionLookupPanel } from "../shared/ActionLookupPanel";
import { SupportingActionsTable } from "../shared/SupportingActionsTable";
import { useExplorerGraphController } from "./hooks/useExplorerGraphController";

export function ExplorerPage() {
  const controller = useExplorerGraphController();
  const [isGraphFullscreen, setIsGraphFullscreen] = useState(false);
  const annotationsQuery = useQuery({
    queryKey: ["annotations"],
    queryFn: listAnnotations,
  });
  const [selectedNamedAddressID, setSelectedNamedAddressID] = useState("");
  const namedAddresses = useMemo(
    () =>
      [...(annotationsQuery.data ?? [])]
        .filter((annotation) => annotation.kind === "label" && annotation.value.trim())
        .sort(
          (left, right) =>
            left.value.localeCompare(right.value) || left.address.localeCompare(right.address)
        ),
    [annotationsQuery.data]
  );

  return (
    <div className="page-stack">
      <div className="page-grid two-up">
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Address Explorer</span>
              <h2>Preview and Load</h2>
            </div>
          </div>
          <form
            className="form-grid"
            onSubmit={(event) => {
              event.preventDefault();
              void controller.requestPreview();
            }}
          >
            <label className="field field-full">
              <span>Address</span>
              <input
                value={controller.form.address}
                onChange={(event) => {
                  setSelectedNamedAddressID("");
                  controller.setForm((current) => ({ ...current, address: event.target.value }));
                }}
                placeholder="thor1..."
              />
            </label>
            <div className="field field-full">
              <span>Named Addresses</span>
              <select
                aria-label="Named Addresses"
                value={selectedNamedAddressID}
                onChange={(event) => {
                  const nextID = event.target.value;
                  setSelectedNamedAddressID(nextID);
                  const annotation = namedAddresses.find((item) => String(item.id) === nextID);
                  if (!annotation) {
                    return;
                  }
                  controller.setForm((current) => ({ ...current, address: annotation.address }));
                }}
                disabled={annotationsQuery.isLoading || !namedAddresses.length}
              >
                <option value="">Choose a saved label annotation</option>
                {namedAddresses.map((annotation) => (
                  <option key={annotation.id} value={String(annotation.id)}>
                    {annotation.value} · {annotation.address}
                  </option>
                ))}
              </select>
              <small>
                Saved `label` annotations appear here. Choosing one fills the explorer address field with its
                underlying address.
              </small>
            </div>
            <label className="field">
              <span>Min USD</span>
              <input
                type="number"
                step="any"
                value={controller.form.min_usd}
                onChange={(event) => controller.setForm((current) => ({ ...current, min_usd: event.target.value }))}
              />
            </label>
            <label className="field">
              <span>Batch Size</span>
              <input
                type="number"
                min={1}
                max={20}
                value={controller.form.batch_size}
                onChange={(event) =>
                  controller.setForm((current) => ({
                    ...current,
                    batch_size: Number(event.target.value) || current.batch_size,
                  }))
                }
              />
            </label>
            <div className="form-actions field-full">
              <button type="submit" className="button" disabled={controller.isPreviewing || controller.isLoadingGraph}>
                {controller.isPreviewing ? "Checking..." : "Preview Address"}
              </button>
            </div>
          </form>
          <p className="status-line">{controller.statusText}</p>
          {controller.preview?.direction_required ? (
            <div className="button-row">
              <button
                type="button"
                className="button secondary"
                disabled={controller.isLoadingGraph}
                onClick={() => {
                  void controller.loadGraph("newest", 0);
                }}
              >
                Load Newest
              </button>
              <button
                type="button"
                className="button secondary"
                disabled={controller.isLoadingGraph}
                onClick={() => {
                  void controller.loadGraph("oldest", 0);
                }}
              >
                Load Oldest
              </button>
            </div>
          ) : null}
          {controller.preview ? (
            <div className="chip-list">
              <span className="meta-chip">{controller.preview.active_chains.length} active chains</span>
              <span className="meta-chip">
                {controller.preview.total_estimate >= 0 ? `${controller.preview.total_estimate} est. actions` : "Total unknown"}
              </span>
              <span className="meta-chip">{formatUSD(controller.preview.query.min_usd)} min</span>
            </div>
          ) : null}
        </section>

        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Runs</span>
              <h2>Saved Explorer Runs</h2>
            </div>
            <span className="status-pill ok">{controller.runs.length}</span>
          </div>
          {controller.isLoadingRuns ? <div className="empty-state">Loading runs…</div> : null}
          <div className="stack">
            <select value={controller.selectedRunID} onChange={(event) => controller.setSelectedRunID(event.target.value)}>
              <option value="">Select a saved run</option>
              {controller.runs.map((run) => (
                <option key={run.id} value={run.id}>
                  {run.summary} · {run.node_count}N/{run.edge_count}E · {formatShortDateTime(run.created_at)}
                </option>
              ))}
            </select>
            <div className="button-row">
              <button
                type="button"
                className="button secondary"
                disabled={!controller.hasSelectedRun}
                onClick={() => {
                  void controller.onLoadRun();
                }}
              >
                Load
              </button>
              <button
                type="button"
                className="button secondary danger"
                disabled={!controller.hasSelectedRun || controller.isDeletingRun}
                onClick={() => {
                  void controller.onDeleteRun();
                }}
              >
                Delete
              </button>
            </div>
          </div>
        </section>
      </div>

      <section className="panel page-panel graph-card-shell">
        <div className="panel-head graph-head">
          <div>
            <span className="eyebrow">Graph</span>
            <h2>{controller.currentGraph ? "Explorer Graph" : "No Graph Loaded"}</h2>
            {controller.currentGraph ? (
              <p>Double-click to expand one edge from the selected node. Right-click nodes for more graph actions.</p>
            ) : null}
          </div>
          <div className="graph-head-actions">
            <GraphStateLoaderButton
              disabled={controller.isPreviewing || controller.isLoadingGraph}
              onLoadFile={(file) => controller.onLoadGraphState(file)}
            />
            {controller.currentGraph ? (
              <div className="graph-stats">
                <span className="meta-chip">
                  {controller.showNodeFraction
                    ? `${controller.visibleNodeCount} / ${controller.totalNodeCount} nodes`
                    : `${controller.totalNodeCount} nodes`}
                </span>
                <span className="meta-chip">
                  {controller.showEdgeFraction
                    ? `${controller.visibleEdgeCount} / ${controller.totalEdgeCount} edges`
                    : `${controller.totalEdgeCount} edges`}
                </span>
                <span className="meta-chip">
                  {controller.showActionFraction
                    ? `${controller.filteredActions.length} / ${controller.totalActionCount} actions`
                    : `${controller.totalActionCount} actions`}
                </span>
              </div>
            ) : null}
          </div>
        </div>

        {controller.currentGraph ? (
          <>
            <div className="chip-list">
              <span className="meta-chip">{shortHash(controller.currentGraph.address)}</span>
              <span className="meta-chip">{controller.currentGraph.query.direction || "newest"} direction</span>
              {controller.expandedHopSeeds.length ? (
                <span className="meta-chip">+{controller.expandedHopSeeds.length} expanded edges</span>
              ) : null}
              {controller.filtersActive ? <span className="meta-chip">Filters active</span> : null}
              <span className="meta-chip">{controller.currentGraph.loaded_actions} loaded actions</span>
            </div>

            {!isGraphFullscreen && controller.currentGraph.warnings.length ? (
              <div className="warning-list">
                {controller.currentGraph.warnings.map((warning) => (
                  <span key={warning} className="warning-chip">
                    {warning}
                  </span>
                ))}
              </div>
            ) : null}

            {controller.visibleGraph && controller.visibleGraph.nodes.length ? (
              <GraphCanvas
                mode="explorer"
                nodes={controller.visibleGraph.nodes}
                edges={controller.visibleGraph.edges}
                selection={controller.selection}
                onSelectionChange={controller.setSelection}
                onNodeDoubleActivate={(node) => {
                  void controller.onExpandNode(node);
                }}
                doubleActivateLabel="Expand one edge"
                graphResetKey={controller.graphResetKey}
                onSaveState={controller.onSaveGraphState}
                savedCanvasState={controller.savedCanvasState}
                onFullscreenChange={setIsGraphFullscreen}
                filters={{
                  isOpen: controller.graphFilters.isOpen,
                  isActive: controller.filtersActive,
                  onToggle: controller.filterActions.toggleOpen,
                  onClose: controller.filterActions.close,
                  content: (
                    <GraphFilterPopover
                      filterState={controller.graphFilters}
                      onToggleTxnType={controller.filterActions.toggleTxnType}
                      onToggleChain={controller.filterActions.toggleChain}
                      onStartTimeChange={(value) => controller.filterActions.updateDate("startTime", value)}
                      onEndTimeChange={(value) => controller.filterActions.updateDate("endTime", value)}
                      onMinUSDChange={(value) => controller.filterActions.updateNumber("minTxnUSD", value)}
                      onMaxUSDChange={(value) => controller.filterActions.updateNumber("maxTxnUSD", value)}
                      onReset={controller.filterActions.resetAllFilters}
                    />
                  ),
                }}
                nodeMenuActions={{
                  onOpenExplorer: (node) => {
                    void controller.nodeActions.onOpenExplorer(node);
                  },
                  onCopyAddress: (node) => {
                    void controller.nodeActions.onCopyAddress(node);
                  },
                  onRefreshLiveValue: (node) => {
                    void controller.nodeActions.onRefreshLiveValue(node);
                  },
                  onExpandNodes: (nodes) => {
                    void controller.onExpandNodes(nodes);
                  },
                  onLabelNode: (node) => {
                    void controller.nodeActions.onLabelNode(node);
                  },
                  onMarkAsgard: (node) => {
                    void controller.nodeActions.onMarkAsgard(node);
                  },
                  onRemoveNode: (node) => {
                    void controller.nodeActions.onRemoveNode(node);
                  },
                }}
                paneMenuActions={{
                  onCheckUnavailable: () => {
                    void controller.nodeActions.onRefreshUnavailable();
                  },
                }}
              />
            ) : (
              <div className="empty-state">
                {controller.filtersActive ? (
                  <>
                    No graph elements match the current filters.{" "}
                    <button
                      type="button"
                      className="button secondary slim"
                      onClick={controller.filterActions.resetAllFilters}
                    >
                      Reset filters
                    </button>
                  </>
                ) : (
                  "No graphable flows found for the selected address."
                )}
              </div>
            )}

            {controller.currentGraph.has_more ? (
              <div className="button-row">
                <button
                  type="button"
                  className="button secondary"
                  disabled={controller.isLoadingGraph}
                  onClick={() => {
                    void controller.loadGraph(
                      (controller.currentGraph?.query.direction || "newest") as "newest" | "oldest",
                      controller.currentGraph?.next_offset || 0
                    );
                  }}
                >
                  {controller.isLoadingGraph ? "Loading..." : "Load Next Batch"}
                </button>
              </div>
            ) : null}
          </>
        ) : (
          <div className="empty-state">Preview an address and load a graph batch to inspect one-hop activity.</div>
        )}
      </section>

      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Inspector</span>
            <h2>Selection Detail</h2>
          </div>
        </div>
        <SelectionInspector
          selection={controller.selection}
          emptyMessage="Select a graph node or edge to inspect explorer graph metadata."
          onLookupTx={controller.onLookup}
        />
      </section>

      <div className="page-grid two-up">
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Actions</span>
              <h2>Supporting Actions</h2>
            </div>
          </div>
          <SupportingActionsTable actions={controller.filteredActions} onLookup={controller.onLookup} />
        </section>

        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Lookup</span>
              <h2>Action Detail</h2>
            </div>
          </div>
          <ActionLookupPanel
            result={controller.lookupResult}
            isLoading={controller.isLookupLoading}
            error={controller.lookupError}
          />
        </section>
      </div>
    </div>
  );
}
