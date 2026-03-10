import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addToBlocklist,
  buildAddressExplorer,
  deleteAddressExplorerRun,
  expandActorGraph,
  listAddressExplorerRuns,
  listAnnotations,
  listBlocklist,
  lookupAction,
  refreshLiveHoldings,
  upsertAnnotation,
} from "../../lib/api";
import { DEFAULT_DISPLAY_MODE, DEFAULT_FLOW_TYPES } from "../../lib/constants";
import { formatShortDateTime, formatUSD, shortHash } from "../../lib/format";
import {
  cloneGraphFilterState,
  createGraphFilterState,
  deriveExplorerVisibleGraph,
  explorerExpansionSeeds,
  explorerURLForAddress,
  filterSupportingActions,
  graphFiltersAreActive,
  mergeAddressExplorerResponse,
  mergeExplorerExpansionResponse,
  nodeAddressForActions,
  rawNodesForVisibleNode,
  resetGraphFilters,
  setGraphFilterDateValue,
  setGraphFilterNumberValue,
  syncGraphFilterStateWithResponse,
  unavailableRawNodes,
  type GraphSelection,
  type GraphTxnBucket,
} from "../../lib/graph";
import type { ActionLookupResponse, AddressExplorerRequest, AddressExplorerResponse } from "../../lib/types";
import { GraphFilterPopover } from "../shared/GraphFilterPopover";
import { GraphCanvas } from "../shared/GraphCanvas";
import { SelectionInspector } from "../shared/SelectionInspector";
import { ActionLookupPanel } from "../shared/ActionLookupPanel";
import { SupportingActionsTable } from "../shared/SupportingActionsTable";

interface ExplorerFormState {
  address: string;
  min_usd: string;
  batch_size: number;
}

function explorerRequest(
  form: ExplorerFormState,
  mode: "preview" | "graph",
  direction: "" | "newest" | "oldest",
  offset: number
): AddressExplorerRequest {
  const minUSD = Number(form.min_usd);
  return {
    address: form.address.trim(),
    flow_types: [...DEFAULT_FLOW_TYPES],
    min_usd: Number.isFinite(minUSD) ? minUSD : 0,
    mode,
    direction,
    offset,
    batch_size: Number(form.batch_size) || 10,
  };
}

function nextFilterState(
  current: ReturnType<typeof createGraphFilterState>,
  graph: AddressExplorerResponse,
  reset = false
) {
  const next = reset ? createGraphFilterState() : cloneGraphFilterState(current);
  syncGraphFilterStateWithResponse(next, graph, { reset });
  return next;
}

export function ExplorerPage() {
  const queryClient = useQueryClient();
  const runsQuery = useQuery({
    queryKey: ["address-explorer-runs"],
    queryFn: listAddressExplorerRuns,
  });
  const annotationsQuery = useQuery({
    queryKey: ["annotations"],
    queryFn: listAnnotations,
  });
  const blocklistQuery = useQuery({
    queryKey: ["blocklist"],
    queryFn: listBlocklist,
  });

  const [form, setForm] = useState<ExplorerFormState>({
    address: "",
    min_usd: "0",
    batch_size: 10,
  });
  const [preview, setPreview] = useState<AddressExplorerResponse | null>(null);
  const [graph, setGraph] = useState<AddressExplorerResponse | null>(null);
  const [selection, setSelection] = useState<GraphSelection>(null);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [statusText, setStatusText] = useState("Enter an address to preview address activity.");
  const [lookupResult, setLookupResult] = useState<ActionLookupResponse | null>(null);
  const [lookupError, setLookupError] = useState("");
  const [graphFilters, setGraphFilters] = useState(createGraphFilterState);
  const [expandedHopSeeds, setExpandedHopSeeds] = useState<string[]>([]);
  const [graphResetKey, setGraphResetKey] = useState(0);

  const previewMutation = useMutation({
    mutationFn: buildAddressExplorer,
    onSuccess: (response) => {
      setPreview(response);
      setGraph(null);
      setSelection(null);
      setLookupResult(null);
      setLookupError("");
      setExpandedHopSeeds([]);
      setGraphFilters(createGraphFilterState());
      setStatusText(
        response.direction_required
          ? "Choose newest or oldest to load the first graph batch."
          : "Preview complete. Loading graph..."
      );
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Explorer preview failed.");
    },
  });

  const graphMutation = useMutation({
    mutationFn: buildAddressExplorer,
    onSuccess: async (response, request) => {
      setPreview(response);
      setGraph((current) => {
        const next = request.offset > 0 ? mergeAddressExplorerResponse(current, response) : response;
        setGraphFilters((filters) => nextFilterState(filters, next, request.offset === 0));
        return next;
      });
      if (request.offset === 0) {
        setGraphResetKey((value) => value + 1);
        setExpandedHopSeeds([]);
        setSelection(null);
      }
      setStatusText(
        request.offset > 0
          ? `Loaded additional actions. ${response.has_more ? "More actions remain." : "No more actions remain."}`
          : `Loaded ${response.nodes.length} nodes and ${response.edges.length} edges for ${response.address}.`
      );
      if (request.offset === 0) {
        await queryClient.invalidateQueries({ queryKey: ["address-explorer-runs"] });
      }
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Explorer graph load failed.");
    },
  });

  const deleteRunMutation = useMutation({
    mutationFn: deleteAddressExplorerRun,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["address-explorer-runs"] });
    },
  });

  const lookupMutation = useMutation({
    mutationFn: lookupAction,
    onSuccess: (response) => {
      setLookupResult(response);
      setLookupError("");
    },
    onError: (error) => {
      setLookupError(error instanceof Error ? error.message : "Action lookup failed.");
    },
  });

  const selectedRun = useMemo(
    () => runsQuery.data?.find((run) => String(run.id) === selectedRunID) ?? null,
    [runsQuery.data, selectedRunID]
  );

  const graphMetadata = useMemo(
    () => ({
      annotations: annotationsQuery.data ?? [],
      blocklist: blocklistQuery.data ?? [],
    }),
    [annotationsQuery.data, blocklistQuery.data]
  );

  const visibleGraph = useMemo(
    () => (graph ? deriveExplorerVisibleGraph(graph, graphFilters, graphMetadata) : null),
    [graph, graphFilters, graphMetadata]
  );

  const filteredActions = useMemo(
    () => (graph ? filterSupportingActions(graph.supporting_actions, graph, graphFilters) : []),
    [graph, graphFilters]
  );

  useEffect(() => {
    if (!visibleGraph || !selection) {
      return;
    }
    const exists =
      selection.kind === "node"
        ? visibleGraph.nodes.some((node) => node.id === selection.node.id)
        : visibleGraph.edges.some((edge) => edge.id === selection.edge.id);
    if (!exists) {
      setSelection(null);
    }
  }, [selection, visibleGraph]);

  function toggleFilterTxnType(bucket: GraphTxnBucket, checked: boolean) {
    setGraphFilters((current) => ({
      ...current,
      txnTypes: {
        ...current.txnTypes,
        [bucket]: checked,
      },
    }));
  }

  function toggleFilterChain(chain: string, checked: boolean) {
    setGraphFilters((current) => ({
      ...current,
      selectedChains: checked
        ? [...new Set([...current.selectedChains, chain])].sort()
        : current.selectedChains.filter((item) => item !== chain),
    }));
  }

  function updateFilterDate(field: "startTime" | "endTime", value: string) {
    setGraphFilters((current) => {
      const next = cloneGraphFilterState(current);
      setGraphFilterDateValue(next, field, value);
      return next;
    });
  }

  function updateFilterNumber(field: "minTxnUSD" | "maxTxnUSD", value: string) {
    setGraphFilters((current) => {
      const next = cloneGraphFilterState(current);
      setGraphFilterNumberValue(next, field, value);
      return next;
    });
  }

  function handleResetFilters() {
    setGraphFilters((current) => {
      const next = cloneGraphFilterState(current);
      resetGraphFilters(next);
      return next;
    });
  }

  async function loadGraph(direction: "newest" | "oldest", offset = 0) {
    const request = explorerRequest(form, "graph", direction, offset);
    setStatusText(offset > 0 ? "Loading more actions..." : "Loading explorer graph...");
    await graphMutation.mutateAsync(request);
  }

  async function requestPreview() {
    const request = explorerRequest(form, "preview", "", 0);
    if (!request.address) {
      setStatusText("Address is required.");
      return;
    }
    const response = await previewMutation.mutateAsync(request);
    if (!response.direction_required) {
      await loadGraph("newest", 0);
    }
  }

  async function handleLoadRun() {
    if (!selectedRun) {
      return;
    }
    setForm({
      address: selectedRun.request.address,
      min_usd: String(selectedRun.request.min_usd ?? 0),
      batch_size: selectedRun.request.batch_size || 10,
    });
    await graphMutation.mutateAsync({
      ...selectedRun.request,
      mode: "graph",
      offset: 0,
      direction: selectedRun.request.direction || "newest",
    });
  }

  async function handleDeleteRun() {
    if (!selectedRun) {
      return;
    }
    await deleteRunMutation.mutateAsync(selectedRun.id);
  }

  async function handleExpandNode(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const seeds = explorerExpansionSeeds(node, graph);
    if (!seeds.length) {
      setStatusText("Selected node has no address context to expand.");
      return;
    }
    const nextSeedSet = [...new Set([...expandedHopSeeds, ...seeds.map((seed) => seed.encoded)])];
    if (nextSeedSet.length === expandedHopSeeds.length) {
      setStatusText("Already expanded from this node.");
      return;
    }
    setStatusText(`Expanding one edge from ${seeds.length} address(es)…`);
    try {
      const expansion = await expandActorGraph({
        actor_ids: [],
        addresses: nextSeedSet,
        start_time: new Date(0).toISOString(),
        end_time: new Date().toISOString(),
        flow_types: [...DEFAULT_FLOW_TYPES],
        min_usd: Number(form.min_usd || 0),
        collapse_external: false,
        display_mode: DEFAULT_DISPLAY_MODE,
      });
      setExpandedHopSeeds(nextSeedSet);
      setGraph((current) => {
        const next = mergeExplorerExpansionResponse(current, expansion);
        setGraphFilters((filters) => nextFilterState(filters, next, false));
        return next;
      });
      setStatusText(`Expanded from ${nextSeedSet.length} address seed(s).`);
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Edge expansion failed.");
    }
  }

  async function handleRefreshNodeLiveValue(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const rawNodes = rawNodesForVisibleNode(node, graph);
    if (!rawNodes.length) {
      setStatusText("Selected node has no live value context.");
      return;
    }
    try {
      const response = await refreshLiveHoldings(rawNodes);
      setGraph((current) => {
        if (!current) {
          return current;
        }
        return {
          ...current,
          nodes: current.nodes.map((rawNode) => {
            const refreshed = response.nodes.find((candidate) => candidate.id === rawNode.id);
            return refreshed
              ? {
                  ...rawNode,
                  metrics: {
                    ...(rawNode.metrics ?? {}),
                    ...(refreshed.metrics ?? {}),
                  },
                }
              : rawNode;
          }),
          warnings: Array.from(new Set([...current.warnings, ...response.warnings])),
        };
      });
      setStatusText(`Refreshed live value for ${rawNodes.length} node(s).`);
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Live value refresh failed.");
    }
  }

  async function handleRefreshUnavailableNodes() {
    if (!graph) {
      return;
    }
    const rawNodes = unavailableRawNodes(graph);
    if (!rawNodes.length) {
      setStatusText("No unavailable nodes.");
      return;
    }
    try {
      const response = await refreshLiveHoldings(rawNodes);
      setGraph((current) => {
        if (!current) {
          return current;
        }
        return {
          ...current,
          nodes: current.nodes.map((rawNode) => {
            const refreshed = response.nodes.find((candidate) => candidate.id === rawNode.id);
            return refreshed
              ? {
                  ...rawNode,
                  metrics: {
                    ...(rawNode.metrics ?? {}),
                    ...(refreshed.metrics ?? {}),
                  },
                }
              : rawNode;
          }),
          warnings: Array.from(new Set([...current.warnings, ...response.warnings])),
        };
      });
      setStatusText(`Checked ${rawNodes.length} unavailable node(s); refreshed ${response.nodes.length}.`);
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Unavailable check failed.");
    }
  }

  async function handleOpenExplorer(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    const url = explorerURLForAddress(address, node.chain);
    if (!url) {
      setStatusText("Selected node does not resolve to a single explorer address.");
      return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
  }

  async function handleCopyAddress(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    await navigator.clipboard.writeText(address);
    setStatusText(`Copied: ${address}`);
  }

  async function handleLabelNode(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    const label = window.prompt("Enter label for this node:", "");
    if (label === null) {
      return;
    }
    await upsertAnnotation({ address, kind: "label", value: label });
    await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    setStatusText(`Saved label for ${address}.`);
  }

  async function handleMarkAsgard(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    await upsertAnnotation({ address, kind: "asgard_vault", value: "true" });
    await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    setStatusText(`Marked ${address} as Asgard.`);
  }

  async function handleRemoveNode(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    await addToBlocklist({ address, reason: "Removed from graph" });
    await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
    setStatusText(`Removed ${address} from graph.`);
  }

  const currentGraph = graph;
  const filtersActive = graphFiltersAreActive(graphFilters);
  const visibleNodeCount = visibleGraph?.nodes.length ?? 0;
  const visibleEdgeCount = visibleGraph?.edges.length ?? 0;
  const totalNodeCount = Number(currentGraph?.stats?.node_count || currentGraph?.nodes.length || 0);
  const totalEdgeCount = Number(currentGraph?.stats?.edge_count || currentGraph?.edges.length || 0);
  const totalActionCount = Number(
    currentGraph?.stats?.supporting_action_count || currentGraph?.supporting_actions.length || 0
  );
  const showNodeFraction = filtersActive || visibleNodeCount !== totalNodeCount;
  const showEdgeFraction = filtersActive || visibleEdgeCount !== totalEdgeCount;
  const showActionFraction = filtersActive || filteredActions.length !== totalActionCount;

  return (
    <div className="page-grid graph-layout">
      <div className="page-stack">
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
              void requestPreview();
            }}
          >
            <label className="field field-full">
              <span>Address</span>
              <input
                value={form.address}
                onChange={(event) => setForm((current) => ({ ...current, address: event.target.value }))}
                placeholder="thor1..."
              />
            </label>
            <label className="field">
              <span>Min USD</span>
              <input
                type="number"
                step="any"
                value={form.min_usd}
                onChange={(event) => setForm((current) => ({ ...current, min_usd: event.target.value }))}
              />
            </label>
            <label className="field">
              <span>Batch Size</span>
              <input
                type="number"
                min={1}
                max={20}
                value={form.batch_size}
                onChange={(event) =>
                  setForm((current) => ({ ...current, batch_size: Number(event.target.value) || current.batch_size }))
                }
              />
            </label>
            <div className="form-actions field-full">
              <button type="submit" className="button" disabled={previewMutation.isPending || graphMutation.isPending}>
                {previewMutation.isPending ? "Checking..." : "Preview Address"}
              </button>
            </div>
          </form>
          <p className="status-line">{statusText}</p>
          {preview?.direction_required ? (
            <div className="button-row">
              <button
                type="button"
                className="button secondary"
                disabled={graphMutation.isPending}
                onClick={() => {
                  void loadGraph("newest", 0);
                }}
              >
                Load Newest
              </button>
              <button
                type="button"
                className="button secondary"
                disabled={graphMutation.isPending}
                onClick={() => {
                  void loadGraph("oldest", 0);
                }}
              >
                Load Oldest
              </button>
            </div>
          ) : null}
          {preview ? (
            <div className="chip-list">
              <span className="meta-chip">{preview.active_chains.length} active chains</span>
              <span className="meta-chip">
                {preview.total_estimate >= 0 ? `${preview.total_estimate} est. actions` : "Total unknown"}
              </span>
              <span className="meta-chip">{formatUSD(preview.query.min_usd)} min</span>
            </div>
          ) : null}
        </section>

        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Runs</span>
              <h2>Saved Explorer Runs</h2>
            </div>
            <span className="status-pill ok">{runsQuery.data?.length ?? 0}</span>
          </div>
          {runsQuery.isLoading ? <div className="empty-state">Loading runs…</div> : null}
          <div className="stack">
            <select value={selectedRunID} onChange={(event) => setSelectedRunID(event.target.value)}>
              <option value="">Select a saved run</option>
              {(runsQuery.data ?? []).map((run) => (
                <option key={run.id} value={run.id}>
                  {run.summary} · {run.node_count}N/{run.edge_count}E · {formatShortDateTime(run.created_at)}
                </option>
              ))}
            </select>
            <div className="button-row">
              <button type="button" className="button secondary" disabled={!selectedRun} onClick={() => void handleLoadRun()}>
                Load
              </button>
              <button
                type="button"
                className="button secondary danger"
                disabled={!selectedRun || deleteRunMutation.isPending}
                onClick={() => void handleDeleteRun()}
              >
                Delete
              </button>
            </div>
          </div>
        </section>
      </div>

      <div className="page-stack">
        <section className="panel page-panel graph-card-shell">
          <div className="panel-head graph-head">
            <div>
              <span className="eyebrow">Graph</span>
              <h2>{currentGraph ? "Explorer Graph" : "No Graph Loaded"}</h2>
              {currentGraph ? (
                <p>Double-click to expand one edge from the selected node. Right-click nodes for more graph actions.</p>
              ) : null}
            </div>
            {currentGraph ? (
              <div className="graph-stats">
                <span className="meta-chip">
                  {showNodeFraction ? `${visibleNodeCount} / ${totalNodeCount} nodes` : `${totalNodeCount} nodes`}
                </span>
                <span className="meta-chip">
                  {showEdgeFraction ? `${visibleEdgeCount} / ${totalEdgeCount} edges` : `${totalEdgeCount} edges`}
                </span>
                <span className="meta-chip">
                  {showActionFraction ? `${filteredActions.length} / ${totalActionCount} actions` : `${totalActionCount} actions`}
                </span>
              </div>
            ) : null}
          </div>

          {currentGraph ? (
            <>
              <div className="chip-list">
                <span className="meta-chip">{shortHash(currentGraph.address)}</span>
                <span className="meta-chip">{currentGraph.query.direction || "newest"} direction</span>
                {expandedHopSeeds.length ? <span className="meta-chip">+{expandedHopSeeds.length} expanded edges</span> : null}
                {filtersActive ? <span className="meta-chip">Filters active</span> : null}
                <span className="meta-chip">{currentGraph.loaded_actions} loaded actions</span>
              </div>

              {currentGraph.warnings.length ? (
                <div className="warning-list">
                  {currentGraph.warnings.map((warning) => (
                    <span key={warning} className="warning-chip">
                      {warning}
                    </span>
                  ))}
                </div>
              ) : null}

              {visibleGraph && visibleGraph.nodes.length ? (
                <GraphCanvas
                  mode="explorer"
                  nodes={visibleGraph.nodes}
                  edges={visibleGraph.edges}
                  selection={selection}
                  onSelectionChange={setSelection}
                  onNodeDoubleActivate={(node) => {
                    void handleExpandNode(node);
                  }}
                  doubleActivateLabel="Expand one edge"
                  graphResetKey={graphResetKey}
                  filterUI={{
                    isOpen: graphFilters.isOpen,
                    isActive: filtersActive,
                    onToggle: () => setGraphFilters((current) => ({ ...current, isOpen: !current.isOpen })),
                    onClose: () => setGraphFilters((current) => ({ ...current, isOpen: false })),
                    popover: (
                      <GraphFilterPopover
                        filterState={graphFilters}
                        onToggleTxnType={toggleFilterTxnType}
                        onToggleChain={toggleFilterChain}
                        onStartTimeChange={(value) => updateFilterDate("startTime", value)}
                        onEndTimeChange={(value) => updateFilterDate("endTime", value)}
                        onMinUSDChange={(value) => updateFilterNumber("minTxnUSD", value)}
                        onMaxUSDChange={(value) => updateFilterNumber("maxTxnUSD", value)}
                        onReset={handleResetFilters}
                      />
                    ),
                  }}
                  onOpenExplorer={(node) => {
                    void handleOpenExplorer(node);
                  }}
                  onCopyAddress={(node) => {
                    void handleCopyAddress(node);
                  }}
                  onRefreshLiveValue={(node) => {
                    void handleRefreshNodeLiveValue(node);
                  }}
                  onLabelNode={(node) => {
                    void handleLabelNode(node);
                  }}
                  onMarkAsgard={(node) => {
                    void handleMarkAsgard(node);
                  }}
                  onRemoveNode={(node) => {
                    void handleRemoveNode(node);
                  }}
                  onCheckUnavailable={() => {
                    void handleRefreshUnavailableNodes();
                  }}
                />
              ) : (
                <div className="empty-state">
                  {filtersActive ? (
                    <>
                      No graph elements match the current filters.{" "}
                      <button type="button" className="button secondary slim" onClick={handleResetFilters}>
                        Reset filters
                      </button>
                    </>
                  ) : (
                    "No graphable flows found for the selected address."
                  )}
                </div>
              )}

              {currentGraph.has_more ? (
                <div className="button-row">
                  <button
                    type="button"
                    className="button secondary"
                    disabled={graphMutation.isPending}
                    onClick={() => {
                      void loadGraph(
                        (currentGraph.query.direction || "newest") as "newest" | "oldest",
                        currentGraph.next_offset
                      );
                    }}
                  >
                    {graphMutation.isPending ? "Loading..." : "Load Next Batch"}
                  </button>
                </div>
              ) : null}
            </>
          ) : (
            <div className="empty-state">Preview an address and load a graph batch to inspect one-hop activity.</div>
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
              actions={filteredActions}
              onLookup={(txID) => lookupMutation.mutate(txID)}
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
              selection={selection}
              emptyMessage="Select a graph node or edge to inspect explorer graph metadata."
              onLookupTx={(txID) => lookupMutation.mutate(txID)}
            />
          </section>

          <section className="panel page-panel">
            <div className="panel-head">
              <div>
                <span className="eyebrow">Lookup</span>
                <h2>Action Detail</h2>
              </div>
            </div>
            <ActionLookupPanel result={lookupResult} isLoading={lookupMutation.isPending} error={lookupError} />
          </section>
        </div>
      </div>
    </div>
  );
}
