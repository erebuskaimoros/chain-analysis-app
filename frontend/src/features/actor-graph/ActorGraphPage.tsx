import { formatDateTime, formatUSD } from "../../lib/format";
import { GraphFilterPopover } from "../shared/GraphFilterPopover";
import { GraphCanvas } from "../shared/GraphCanvas";
import { SelectionInspector } from "../shared/SelectionInspector";
import { ActionLookupPanel } from "../shared/ActionLookupPanel";
import { SupportingActionsTable } from "../shared/SupportingActionsTable";
import { ActorGraphSidebar } from "./ActorGraphSidebar";
import { useActorGraphController } from "./hooks/useActorGraphController";

export function ActorGraphPage() {
  const controller = useActorGraphController();

  return (
    <div className="page-grid graph-layout">
      <ActorGraphSidebar
        actors={controller.actorOptions}
        selectedActorIDs={controller.selectedActorIDs}
        onToggleActor={controller.toggleActor}
        form={controller.form}
        onFormChange={controller.setForm}
        onBuild={() => {
          void controller.onBuild();
        }}
        isBuilding={controller.isBuilding}
        canBuild={controller.canBuild}
        onRefreshLiveHoldings={() => {
          void controller.onRefreshAllLiveHoldings();
        }}
        canRefreshLiveHoldings={Boolean(controller.graph)}
        isRefreshingLiveHoldings={false}
        statusText={controller.statusText}
        runs={controller.runs}
        selectedRunID={controller.selectedRunID}
        onSelectedRunIDChange={controller.setSelectedRunID}
        onLoadRun={() => {
          void controller.onLoadRun();
        }}
        onDeleteRun={() => {
          void controller.onDeleteRun();
        }}
        isDeletingRun={controller.isDeletingRun}
        hasSelectedRun={controller.hasSelectedRun}
        isLoadingRuns={controller.isLoadingRuns}
      />

      <div className="page-stack">
        <section className="panel page-panel graph-card-shell">
          <div className="panel-head graph-head">
            <div>
              <span className="eyebrow">Graph</span>
              <h2>{controller.graph ? "Current Flow Graph" : "No Graph Loaded"}</h2>
              {controller.graph ? (
                <p>
                  Click actor nodes to expand owned addresses. Click external clusters to expand one chain. Double-click to
                  expand one hop. Right-click nodes for more actions.
                </p>
              ) : null}
            </div>
            {controller.graph ? (
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

          {controller.graph ? (
            <>
              <div className="chip-list">
                <span className="meta-chip">{controller.graph.actors.length} actors</span>
                <span className="meta-chip">{controller.graph.query.max_hops} hops</span>
                <span className="meta-chip">
                  {formatDateTime(controller.graph.query.start_time)} → {formatDateTime(controller.graph.query.end_time)}
                </span>
                <span className="meta-chip">
                  {controller.graph.query.coverage_satisfied ? "Full cache coverage" : "Partial cache coverage"}
                </span>
                <span className="meta-chip">{controller.graph.query.blocks_scanned} blocks scanned</span>
                {controller.expandedHopSeeds.length ? (
                  <span className="meta-chip">+{controller.expandedHopSeeds.length} one-hop seeds</span>
                ) : null}
                {controller.filtersActive ? <span className="meta-chip">Filters active</span> : null}
                <span className="meta-chip">{formatUSD(controller.graph.query.min_usd)} min</span>
              </div>

              {controller.graph.warnings.length ? (
                <div className="warning-list">
                  {controller.graph.warnings.map((warning) => (
                    <span key={warning} className="warning-chip">
                      {warning}
                    </span>
                  ))}
                </div>
              ) : null}

              {controller.visibleGraph && controller.visibleGraph.nodes.length ? (
                <GraphCanvas
                  mode="actor"
                  nodes={controller.visibleGraph.nodes}
                  edges={controller.visibleGraph.edges}
                  selection={controller.selection}
                  onSelectionChange={controller.setSelection}
                  onNodePrimaryAction={controller.onNodePrimaryAction}
                  onNodeDoubleActivate={(node) => {
                    void controller.onExpandNode(node);
                  }}
                  graphResetKey={controller.graphResetKey}
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
                    "No graphable flows found for the selected actors and time window."
                  )}
                </div>
              )}
            </>
          ) : (
            <div className="empty-state">Build a graph to inspect relationships and supporting actions.</div>
          )}
        </section>

        <div className="page-grid inspector-grid">
          <section className="panel page-panel">
            <div className="panel-head">
              <div>
                <span className="eyebrow">Actions</span>
                <h2>Supporting Actions</h2>
              </div>
            </div>
            <SupportingActionsTable
              actions={controller.filteredActions}
              onLookup={controller.onLookup}
            />
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
              emptyMessage="Select a graph node or edge to inspect labels, metrics, and transactions."
              onLookupTx={controller.onLookup}
            />
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
    </div>
  );
}
