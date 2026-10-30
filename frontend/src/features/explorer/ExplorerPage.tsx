import { lazy, Suspense, useEffect, useId, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertIcon, PanelLeftIcon, PlayIcon, StopIcon, TrashIcon } from "../../app/icons";
import { useActiveCase } from "../../app/activeCase";
import { useRouteIntent } from "../../app/router";
import { listAnnotations } from "../../lib/api";
import { formatRelativeTime, middleTruncate, pluralize } from "../../lib/format";
import { identifyInput } from "../../lib/identify";
import { ChainBadge } from "../../ui/AddressText";
import { Field } from "../../ui/Field";
import { MenuButton } from "../../ui/Menu";
import { PageHeader } from "../../ui/PageHeader";
import { GraphFilterPopover } from "../shared/GraphFilterPopover";
import { GraphStateLoaderButton } from "../shared/GraphStateLoaderButton";
import { GraphTimeScrubber } from "../shared/GraphTimeScrubber";
import { SupportingActionsTable } from "../shared/SupportingActionsTable";
import { GraphWorkspace } from "../shared/workspace/GraphWorkspace";
import { GraphInspector, InspectorTabs, inspectorTitle, type InspectorTab } from "../shared/workspace/GraphInspector";
import { MapMessage, MapNotice, MapProgress, MapStatus, MapWarnings } from "../shared/workspace/MapOverlays";
import { useExplorerGraphController } from "./hooks/useExplorerGraphController";

const GraphCanvas = lazy(() => import("../shared/GraphCanvas").then((module) => ({ default: module.GraphCanvas })));

const RECENT_LIMIT = 6;

export function ExplorerPage() {
  const controller = useExplorerGraphController();
  const { addToCase } = useActiveCase();
  const annotationsQuery = useQuery({ queryKey: ["annotations"], queryFn: listAnnotations });
  const [queryOpen, setQueryOpen] = useState(true);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorTab, setInspectorTab] = useState<InspectorTab>("details");
  const [labelingNodeID, setLabelingNodeID] = useState<string | null>(null);
  const [isGraphFullscreen, setIsGraphFullscreen] = useState(false);
  const [warningsHidden, setWarningsHidden] = useState(false);
  const [showAllRuns, setShowAllRuns] = useState(false);
  const lastResetKeyRef = useRef(controller.graphResetKey);
  const datalistID = useId();

  const graph = controller.currentGraph;
  const selection = controller.selection;
  const isWorking = controller.isPreviewing || controller.isLoadingGraph;

  useRouteIntent("explorer", (params) => {
    controller.applyIntent(params);
    if (params.tx) {
      setInspectorOpen(true);
      setInspectorTab("tx");
    }
  });

  useEffect(() => {
    if (controller.graphResetKey !== lastResetKeyRef.current) {
      lastResetKeyRef.current = controller.graphResetKey;
      // Fold the panel away only when there is something to look at; an
      // empty result usually means adjusting the query.
      if (controller.currentGraph && controller.visibleGraph?.nodes.length) {
        setQueryOpen(false);
      }
      setWarningsHidden(false);
    }
  }, [controller.currentGraph, controller.graphResetKey, controller.visibleGraph]);

  useEffect(() => {
    if (selection) {
      setInspectorOpen(true);
      setInspectorTab("details");
    }
    setLabelingNodeID((current) => (selection?.kind === "node" && selection.node.id === current ? current : null));
  }, [selection]);

  const labeledAddresses = useMemo(
    () =>
      [...(annotationsQuery.data ?? [])]
        .filter((annotation) => annotation.kind === "label" && annotation.value.trim())
        .sort((left, right) => left.value.localeCompare(right.value) || left.address.localeCompare(right.address)),
    [annotationsQuery.data]
  );
  const labelByAddress = useMemo(
    () => new Map(labeledAddresses.map((annotation) => [annotation.address.toLowerCase(), annotation.value])),
    [labeledAddresses]
  );
  const nodeLabelByID = useMemo(
    () => new Map((controller.visibleGraph?.nodes ?? []).map((node) => [node.id, node.displayLabel || node.label])),
    [controller.visibleGraph]
  );

  const typed = identifyInput(controller.form.address);
  const typedLabel = labelByAddress.get(controller.form.address.trim().toLowerCase());
  const hasNodes = Boolean(controller.visibleGraph?.nodes.length);
  const warnings = graph?.warnings ?? [];
  const showWarnings = warnings.length > 0 && !warningsHidden && !isGraphFullscreen;
  const preview = controller.preview;
  const runs = showAllRuns ? controller.runs : controller.runs.slice(0, RECENT_LIMIT);

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

  const context = graph
    ? [
        labelByAddress.get(graph.address.toLowerCase()) || middleTruncate(graph.address, 10, 6),
        `${graph.query.direction || "newest"} first`,
        `${graph.loaded_actions} actions loaded`,
      ].join(", ")
    : isWorking
      ? "Loading…"
      : "";

  const queryPanel = (
    <>
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          void controller.requestPreview();
        }}
      >
        <Field
          label="Address"
          hint={
            typed.kind === "address"
              ? `${typedLabel ? `${typedLabel}, ` : ""}${typed.chainCertain ? `${typed.chain} address` : `${typed.chain}-style address`}`
              : typed.kind === "tx"
                ? "That is a transaction hash. Search (⌘K) looks transactions up."
                : labeledAddresses.length
                  ? "Type a label to pick from your labeled addresses."
                  : "Any THORChain, MAYA or connected-chain address."
          }
        >
          <input
            className="mono"
            value={controller.form.address}
            list={datalistID}
            onChange={(event) => controller.setForm((current) => ({ ...current, address: event.target.value }))}
            placeholder="thor1..."
            spellCheck={false}
            autoComplete="off"
          />
        </Field>
        <datalist id={datalistID}>
          {labeledAddresses.map((annotation) => (
            <option key={annotation.id} value={annotation.address} label={annotation.value} />
          ))}
        </datalist>

        <details className="disclosure">
          <summary>More options</summary>
          <div className="disclosure-body">
            <div className="field-row">
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
                <span>Batch size</span>
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
            </div>
            <small className="field-hint">Batch size is how many actions each load fetches.</small>
          </div>
        </details>

        <div className="form-actions">
          {isWorking ? (
            <button type="button" className="btn" onClick={controller.cancelJob}>
              <StopIcon />
              Cancel
            </button>
          ) : (
            <button type="submit" className="btn btn-primary" disabled={!controller.form.address.trim()}>
              Explore address
            </button>
          )}
        </div>
        <p className="status-text" role="status">
          {controller.statusText}
        </p>
      </form>

      {preview ? (
        <section className="ws-group" aria-label="Address summary">
          <h3>What we found</h3>
          <div className="chip-set">
            {preview.active_chains.map((chain) => (
              <ChainBadge key={chain} chain={chain} />
            ))}
          </div>
          <p className="section-note">
            {preview.total_estimate >= 0 ? `About ${pluralize(preview.total_estimate, "action")}` : "Action count unknown"}
            {preview.active_chains.length ? ` across ${pluralize(preview.active_chains.length, "chain")}` : ""}.
          </p>
          {preview.direction_required ? (
            <div className="empty-state">
              This address has a long history. Start from the newest actions or the oldest, then load more in batches.
              <div className="form-actions">
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  disabled={isWorking}
                  onClick={() => {
                    void controller.loadGraph("newest", 0);
                  }}
                >
                  Load newest
                </button>
                <button
                  type="button"
                  className="btn btn-sm"
                  disabled={isWorking}
                  onClick={() => {
                    void controller.loadGraph("oldest", 0);
                  }}
                >
                  Load oldest
                </button>
              </div>
            </div>
          ) : null}
        </section>
      ) : null}

      <section className="ws-group" aria-label="Recent explorations">
        <div className="ws-group-head">
          <h3>Recent explorations</h3>
          {controller.runs.length > RECENT_LIMIT ? (
            <button type="button" className="link-btn" onClick={() => setShowAllRuns((value) => !value)}>
              {showAllRuns ? "Show fewer" : `Show all ${controller.runs.length}`}
            </button>
          ) : null}
        </div>
        {controller.isLoadingRuns ? <p className="status-text">Loading…</p> : null}
        {controller.runs.length ? (
          <ul className="list ws-run-list">
            {runs.map((run) => (
              <li key={run.id} className="list-row">
                <span className="list-row-main">
                  <span className="list-row-title" title={run.request.address}>
                    {labelByAddress.get(run.request.address.toLowerCase()) || run.summary || middleTruncate(run.request.address)}
                  </span>
                  <span className="list-row-meta">
                    {pluralize(run.node_count, "node")}, {formatRelativeTime(run.created_at)}
                  </span>
                </span>
                <button
                  type="button"
                  className="btn btn-sm"
                  disabled={isWorking}
                  onClick={() => {
                    void controller.onRunAgain(run);
                  }}
                >
                  <PlayIcon size={12} />
                  Open
                </button>
                <MenuButton
                  label={`Actions for ${run.summary || run.request.address}`}
                  items={[
                    {
                      label: "Delete",
                      icon: <TrashIcon />,
                      danger: true,
                      onSelect: () => {
                        void controller.onDeleteRun(run);
                      },
                    },
                  ]}
                />
              </li>
            ))}
          </ul>
        ) : !controller.isLoadingRuns ? (
          <p className="section-note">Addresses you explore are listed here.</p>
        ) : null}
      </section>

      <section className="ws-group">
        <div className="ws-group-head">
          <h3>Saved explorer graphs</h3>
          <GraphStateLoaderButton
            className="btn btn-ghost btn-sm"
            label="Open a file…"
            disabled={isWorking}
            onLoadFile={(file) => controller.onLoadGraphState(file)}
          />
        </div>
        <p className="section-note">Explorer graphs save as files from the map toolbar. Load one here to pick up where you left off.</p>
      </section>
    </>
  );

  const canvas = (
    <div className="graph-card-shell">
      {graph && hasNodes ? (
        <Suspense fallback={<div className="graph-surface" />}>
          <GraphCanvas
            mode="explorer"
            nodes={controller.visibleGraph!.nodes}
            edges={controller.visibleGraph!.edges}
            selection={selection}
            onSelectionChange={controller.setSelection}
            onNodeDoubleActivate={(node) => {
              void controller.onExpandNode(node);
            }}
            doubleActivateLabel="Expand one edge"
            graphResetKey={controller.graphResetKey}
            onSaveState={controller.onSaveGraphState}
            defaultSaveName={graph.address || "address-explorer state"}
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

      {isWorking ? (
        <MapProgress
          title={controller.isPreviewing ? "Checking the address" : "Loading actions"}
          progress={controller.progress}
          onCancel={controller.cancelJob}
          variant={graph && hasNodes ? "banner" : "card"}
        />
      ) : controller.isExpanding ? (
        <MapProgress title="Expanding" progress={{ stage: "fetching neighbours", done: 0, total: 0 }} variant="banner" />
      ) : null}

      {!isWorking && !graph ? (
        preview?.direction_required ? (
          <MapMessage
            title="Choose where to start"
            actions={
              <>
                <button type="button" className="btn btn-primary" onClick={() => void controller.loadGraph("newest", 0)}>
                  Load newest
                </button>
                <button type="button" className="btn" onClick={() => void controller.loadGraph("oldest", 0)}>
                  Load oldest
                </button>
              </>
            }
          >
            This address has a long history. Load the newest or the oldest actions first, then more in batches.
          </MapMessage>
        ) : (
          <MapMessage
            title="Explore an address"
            actions={
              queryOpen ? null : (
                <button type="button" className="btn btn-primary" onClick={() => setQueryOpen(true)}>
                  Open the query panel
                </button>
              )
            }
          >
            Paste an address in the query panel to map who it sent to and received from. Search (⌘K) works from any page.
          </MapMessage>
        )
      ) : null}

      {!isWorking && graph && !hasNodes ? (
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
          <MapMessage title="No flows found">No graphable flows found for the selected address.</MapMessage>
        )
      ) : null}

      {graph ? (
        <MapStatus>
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

      {graph?.has_more && !isWorking ? (
        <div className="map-overlay map-bottom-center">
          <button
            type="button"
            className="map-pill is-accent"
            onClick={() => {
              void controller.loadGraph(
                (graph.query.direction || "newest") as "newest" | "oldest",
                graph.next_offset || 0
              );
            }}
          >
            Load the next {controller.form.batch_size} actions
          </button>
        </div>
      ) : null}

      {showWarnings ? <MapWarnings warnings={warnings} onClose={() => setWarningsHidden(true)} /> : null}

      {!queryOpen && !isWorking ? <MapNotice text={controller.statusText} /> : null}

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
        title="Explorer"
        context={context}
        actions={
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
        }
      />
      <GraphWorkspace
        left={{
          title: "Query",
          open: queryOpen,
          onClose: () => setQueryOpen(false),
          closeLabel: "Hide the query panel",
          content: queryPanel,
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
                storageKey: "chain-analysis.explorer-tray",
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
                    onAddToCase={(action) => addToCase([{ kind: "tx", ref: action.tx_id }], middleTruncate(action.tx_id))}
                  />
                ),
              }
            : undefined
        }
      />
    </>
  );
}
