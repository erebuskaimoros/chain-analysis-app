import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { AlertIcon, PanelLeftIcon, RefreshIcon } from "../../app/icons";
import { useActiveCase } from "../../app/activeCase";
import { useRouteIntent } from "../../app/router";
import { formatDateRange, middleTruncate, pluralize } from "../../lib/format";
import { PageHeader } from "../../ui/PageHeader";
import { GraphFilterPopover } from "../shared/GraphFilterPopover";
import { GraphTimeScrubber } from "../shared/GraphTimeScrubber";
import { SupportingActionsTable } from "../shared/SupportingActionsTable";
import { useGraphMetadata } from "../shared/graph-hooks/useGraphMetadata";
import { GraphWorkspace } from "../shared/workspace/GraphWorkspace";
import { GraphInspector, InspectorTabs, inspectorTitle, type InspectorTab } from "../shared/workspace/GraphInspector";
import { MapMessage, MapNotice, MapProgress, MapStatus, MapWarnings } from "../shared/workspace/MapOverlays";
import { ActorGraphSidebar } from "./ActorGraphSidebar";
import { useActorGraphController } from "./hooks/useActorGraphController";

const GraphCanvas = lazy(() => import("../shared/GraphCanvas").then((module) => ({ default: module.GraphCanvas })));

export function ActorGraphPage() {
  const controller = useActorGraphController();
  const { addToCase } = useActiveCase();
  const { metadata } = useGraphMetadata();
  const [queryOpen, setQueryOpen] = useState(true);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorTab, setInspectorTab] = useState<InspectorTab>("details");
  const [labelingNodeID, setLabelingNodeID] = useState<string | null>(null);
  const [isGraphFullscreen, setIsGraphFullscreen] = useState(false);
  const [warningsHidden, setWarningsHidden] = useState(false);
  const lastResetKeyRef = useRef(controller.graphResetKey);

  useRouteIntent("graph", (params) => {
    setQueryOpen(true);
    controller.applyIntent(params);
  });

  const graph = controller.graph;
  const selection = controller.selection;

  // A finished graph takes the screen: fold the query panel away. It comes
  // back from the header.
  useEffect(() => {
    if (controller.graphResetKey !== lastResetKeyRef.current) {
      lastResetKeyRef.current = controller.graphResetKey;
      // Fold the panel away only when there is something to look at; an
      // empty result usually means adjusting the query.
      if (controller.graph && controller.visibleGraph?.nodes.length) {
        setQueryOpen(false);
      }
      setWarningsHidden(false);
    }
  }, [controller.graph, controller.graphResetKey, controller.visibleGraph]);

  useEffect(() => {
    if (selection) {
      setInspectorOpen(true);
      setInspectorTab("details");
    }
    setLabelingNodeID((current) => (selection?.kind === "node" && selection.node.id === current ? current : null));
  }, [selection]);

  const nodeLabelByID = useMemo(
    () => new Map((controller.visibleGraph?.nodes ?? []).map((node) => [node.id, node.displayLabel || node.label])),
    [controller.visibleGraph]
  );
  const actorNameByID = useMemo(
    () => new Map(controller.actorOptions.map((actor) => [actor.id, actor.name])),
    [controller.actorOptions]
  );
  const labelByAddress = useMemo(
    () =>
      new Map(
        metadata.annotations
          .filter((annotation) => annotation.kind === "label")
          .map((annotation) => [annotation.address.toLowerCase(), annotation.value])
      ),
    [metadata.annotations]
  );

  function lookup(txID: string) {
    controller.onLookup(txID);
    setInspectorOpen(true);
    setInspectorTab("tx");
  }

  function closeInspector() {
    setInspectorOpen(false);
    setLabelingNodeID(null);
    controller.setSelection(null);
  }

  const hasNodes = Boolean(controller.visibleGraph?.nodes.length);
  const warnings = graph?.warnings ?? [];
  const showWarnings = warnings.length > 0 && !warningsHidden && !isGraphFullscreen;
  const context = graph
    ? [
        graph.actors.map((actor) => actor.name).join(", ") || "Selected actors",
        formatDateRange(graph.query.start_time, graph.query.end_time),
        pluralize(graph.query.max_hops, "hop"),
      ]
        .filter(Boolean)
        .join(", ")
    : controller.isBuilding
      ? "Building a graph…"
      : "";

  const canvas = (
    <div className="graph-card-shell">
      {graph && hasNodes ? (
        <Suspense fallback={<div className="graph-surface" />}>
          <GraphCanvas
            mode="actor"
            nodes={controller.visibleGraph!.nodes}
            edges={controller.visibleGraph!.edges}
            selection={selection}
            onSelectionChange={controller.setSelection}
            onNodePrimaryAction={controller.onNodePrimaryAction}
            nodeHasPrimaryAction={(node) =>
              (node.kind === "actor" && node.actor_ids.length === 1) ||
              (node.kind === "external_cluster" && Boolean(node.chain))
            }
            onNodeDoubleActivate={(node) => {
              void controller.onExpandNode(node);
            }}
            graphResetKey={controller.graphResetKey}
            onSaveState={controller.isPartialGraph ? undefined : controller.onSaveGraphState}
            defaultSaveName={controller.defaultSaveStateName}
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
                  labelCategories={controller.labelCategories}
                  onToggleLabelCategory={controller.filterActions.toggleLabelCategory}
                  onStartTimeChange={(value) => controller.filterActions.updateDate("startTime", value)}
                  onEndTimeChange={(value) => controller.filterActions.updateDate("endTime", value)}
                  onMinUSDChange={(value) => controller.filterActions.updateNumber("minTxnUSD", value)}
                  onMaxUSDChange={(value) => controller.filterActions.updateNumber("maxTxnUSD", value)}
                  onReset={controller.filterActions.resetAllFilters}
                />
              ),
            }}
            nodeMenuActions={{
              onExploreAddress: controller.nodeActions.onExploreAddress,
              onTraceFrom: controller.nodeActions.onTraceFrom,
              onAddToCase: controller.nodeActions.onAddToCase,
              onAddNodesToCase: controller.nodeActions.onAddNodesToCase,
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
                controller.setSelection({ kind: "node", node });
                setLabelingNodeID(node.id);
                setInspectorOpen(true);
                setInspectorTab("details");
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
        </Suspense>
      ) : (
        <div className="graph-surface" />
      )}

      {controller.isBuilding ? (
        <MapProgress
          title="Building the graph"
          progress={controller.buildProgress}
          onCancel={controller.cancelBuild}
          variant={graph && hasNodes ? "banner" : "card"}
        />
      ) : controller.isExpanding ? (
        <MapProgress
          title="Expanding"
          progress={{ stage: "fetching neighbours", done: 0, total: 0 }}
          variant="banner"
        />
      ) : null}

      {!controller.isBuilding && !graph ? (
        <MapMessage
          title="Build a graph from your actors"
          actions={
            queryOpen ? null : (
              <button type="button" className="btn btn-primary" onClick={() => setQueryOpen(true)}>
                Open the query panel
              </button>
            )
          }
        >
          Pick actors and a time window in the query panel, then build. Saved graphs and recent builds are listed there too.
        </MapMessage>
      ) : null}

      {!controller.isBuilding && graph && !hasNodes ? (
        controller.filtersActive ? (
          <MapMessage
            title="Nothing matches the filters"
            actions={
              <button type="button" className="btn" onClick={controller.filterActions.resetAllFilters}>
                Reset filters
              </button>
            }
          >
            No graph elements match the current filters.
          </MapMessage>
        ) : (
          <MapMessage title="No flows to draw">
            The build found no flows to show for these actors in this window. Try a longer window or more hops.
          </MapMessage>
        )
      ) : null}

      {graph ? (
        <MapStatus>
          {controller.isPartialGraph ? <span className="map-pill is-accent">Partial graph</span> : null}
          {controller.isRefreshingLiveHoldings ? (
            <span className="map-pill">
              <span className="spinner" aria-hidden="true" /> Updating live values
            </span>
          ) : null}
          {!graph.query.coverage_satisfied && !controller.isPartialGraph ? (
            <span className="map-pill is-warn" title={`${graph.query.blocks_scanned} blocks scanned`}>
              Partial cache coverage
            </span>
          ) : null}
          {warnings.length && (warningsHidden || isGraphFullscreen) ? (
            <button type="button" className="map-pill is-warn" onClick={() => setWarningsHidden(false)}>
              <AlertIcon size={12} />
              {pluralize(warnings.length, "warning")}
            </button>
          ) : null}
          {controller.filtersActive ? (
            <button type="button" className="map-pill is-accent" onClick={controller.filterActions.resetAllFilters}>
              Filters on, reset
            </button>
          ) : null}
          <span className="map-pill">
            {controller.showNodeFraction
              ? `${controller.visibleNodeCount} of ${controller.totalNodeCount} nodes`
              : pluralize(controller.totalNodeCount, "node")}
            , {controller.showEdgeFraction
              ? `${controller.visibleEdgeCount} of ${controller.totalEdgeCount} edges`
              : pluralize(controller.totalEdgeCount, "edge")}
          </span>
        </MapStatus>
      ) : null}

      {showWarnings ? <MapWarnings warnings={warnings} onClose={() => setWarningsHidden(true)} /> : null}

      {!queryOpen && !controller.isBuilding ? <MapNotice text={controller.statusText} /> : null}

      {graph && hasNodes && !isGraphFullscreen ? (
        <GraphTimeScrubber
          filterState={controller.graphFilters}
          onStartTimeChange={(value) => controller.filterActions.updateDate("startTime", value)}
          onEndTimeChange={(value) => controller.filterActions.updateDate("endTime", value)}
        />
      ) : null}
    </div>
  );

  return (
    <>
      <PageHeader
        title="Actor graph"
        context={context}
        actions={
          <>
            {graph ? (
              <button
                type="button"
                className="btn btn-sm"
                disabled={controller.isRefreshingLiveHoldings || controller.isPartialGraph}
                onClick={() => {
                  void controller.onRefreshAllLiveHoldings();
                }}
                title="Look up what every address in the graph holds now"
              >
                <RefreshIcon />
                {controller.isRefreshingLiveHoldings ? "Refreshing…" : "Refresh holdings"}
              </button>
            ) : null}
            <button
              type="button"
              className="btn btn-sm"
              aria-pressed={queryOpen}
              onClick={() => setQueryOpen((value) => !value)}
              title={queryOpen ? "Hide the query panel" : "Show the query panel"}
            >
              <PanelLeftIcon />
              Query
            </button>
          </>
        }
      />
      <GraphWorkspace
        left={{
          title: "Query",
          open: queryOpen,
          onClose: () => setQueryOpen(false),
          closeLabel: "Hide the query panel",
          content: (
            <ActorGraphSidebar
              actors={controller.actorOptions}
              actorsLoading={controller.actorsLoading}
              selectedActorIDs={controller.selectedActorIDs}
              onToggleActor={controller.toggleActor}
              onClearActors={() => controller.setSelectedActorIDs([])}
              form={controller.form}
              onFormChange={controller.setForm}
              onBuild={() => {
                void controller.onBuild();
              }}
              onCancelBuild={controller.cancelBuild}
              isBuilding={controller.isBuilding}
              canBuild={controller.canBuild}
              statusText={controller.statusText}
              runs={controller.runs}
              onRunAgain={(run) => {
                void controller.onRunAgain(run);
              }}
              onDeleteRun={(run) => {
                void controller.onDeleteRun(run);
              }}
              isLoadingRuns={controller.isLoadingRuns}
              savedStates={controller.savedStates}
              onOpenSavedState={(state) => {
                void controller.onOpenSavedState(state);
              }}
              onDeleteSavedState={(state) => {
                void controller.onDeleteSavedState(state);
              }}
              onExportSavedState={(state) => {
                void controller.onExportSavedState(state);
              }}
              onLoadGraphFile={(file) => {
                void controller.onLoadGraphState(file);
              }}
              isLoadingSavedStates={controller.isLoadingSavedStates}
            />
          ),
        }}
        right={{
          title: inspectorTitle(selection, inspectorTab),
          open: inspectorOpen && Boolean(selection || controller.lookupTxID),
          onClose: closeInspector,
          closeLabel: "Close the inspector",
          subheader: (
            <InspectorTabs
              tab={inspectorTab}
              onTabChange={setInspectorTab}
              hasLookup={Boolean(controller.lookupTxID) && Boolean(selection)}
            />
          ),
          content: (
            <GraphInspector
              selection={selection}
              tab={selection ? inspectorTab : "tx"}
              lookup={{
                result: controller.lookupResult,
                isLoading: controller.isLookupLoading,
                error: controller.lookupError,
                txID: controller.lookupTxID,
              }}
              onLookupTx={lookup}
              nodeLabel={(id) => nodeLabelByID.get(id) ?? id}
              actorName={(id) => actorNameByID.get(id) ?? `Actor ${id}`}
              nodeActions={controller.nodeActions}
              onExpandNode={(node) => {
                void controller.onExpandNode(node);
              }}
              onExpandNodes={(nodes) => {
                void controller.onExpandNodes(nodes);
              }}
              labelingNodeID={labelingNodeID}
              onLabelingChange={setLabelingNodeID}
              existingLabel={(address) => labelByAddress.get(address.toLowerCase()) ?? ""}
            />
          ),
        }}
        canvas={canvas}
        tray={
          graph
            ? {
                title: "Supporting actions",
                count: controller.filteredActions.length,
                storageKey: "chain-analysis.graph-tray",
                extra: controller.showActionFraction ? (
                  <span>
                    {controller.filteredActions.length} of {controller.totalActionCount} after filters
                  </span>
                ) : null,
                content: (
                  <SupportingActionsTable
                    actions={controller.filteredActions}
                    onLookup={lookup}
                    selectedTxID={controller.lookupTxID}
                    onAddToCase={(action) =>
                      addToCase([{ kind: "tx", ref: action.tx_id }], middleTruncate(action.tx_id))
                    }
                  />
                ),
              }
            : undefined
        }
      />
    </>
  );
}
