import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addToBlocklist,
  buildActorGraph,
  deleteActorGraphRun,
  expandActorGraph,
  listActorGraphRuns,
  listActors,
  listAnnotations,
  listBlocklist,
  lookupAction,
  refreshLiveHoldings,
  upsertAnnotation,
} from "../../lib/api";
import { DEFAULT_DISPLAY_MODE, DEFAULT_FLOW_TYPES, defaultActorGraphWindow } from "../../lib/constants";
import { formatDateTime, formatShortDateTime, formatUSD, toLocalInputValue } from "../../lib/format";
import {
  actorExpansionSeeds,
  applyNodeUpdates,
  cloneGraphFilterState,
  createGraphFilterState,
  deriveActorVisibleGraph,
  explorerURLForAddress,
  filterSupportingActions,
  graphFiltersAreActive,
  mergeActorGraphResponse,
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
import type {
  ActionLookupResponse,
  Actor,
  ActorGraphRequest,
  ActorGraphResponse,
} from "../../lib/types";
import { GraphFilterPopover } from "../shared/GraphFilterPopover";
import { GraphCanvas } from "../shared/GraphCanvas";
import { SelectionInspector } from "../shared/SelectionInspector";
import { ActionLookupPanel } from "../shared/ActionLookupPanel";
import { SupportingActionsTable } from "../shared/SupportingActionsTable";
import { ActorGraphSidebar, type GraphFormState } from "./ActorGraphSidebar";

function defaultFormState(): GraphFormState {
  const window = defaultActorGraphWindow();
  return {
    start_time: toLocalInputValue(window.start),
    end_time: toLocalInputValue(window.end),
    max_hops: 4,
    min_usd: "0",
  };
}

function requestFromState(form: GraphFormState, actorIDs: number[]): ActorGraphRequest {
  const minUSD = Number(form.min_usd);
  return {
    actor_ids: actorIDs,
    start_time: form.start_time,
    end_time: form.end_time,
    max_hops: Number(form.max_hops) || 4,
    flow_types: [...DEFAULT_FLOW_TYPES],
    min_usd: Number.isFinite(minUSD) ? minUSD : 0,
    collapse_external: false,
    display_mode: DEFAULT_DISPLAY_MODE,
  };
}

function stateFromRequest(request: ActorGraphRequest): GraphFormState {
  return {
    start_time: request.start_time,
    end_time: request.end_time,
    max_hops: request.max_hops || 4,
    min_usd: String(request.min_usd ?? 0),
  };
}

function actorNames(actors: Actor[]) {
  return actors.map((actor) => actor.name).join(", ");
}

function toggleNumber(values: number[], value: number) {
  return values.includes(value) ? values.filter((item) => item !== value) : [...values, value];
}

function toggleString(values: string[], value: string) {
  return values.includes(value) ? values.filter((item) => item !== value) : [...values, value];
}

function nextFilterState(
  current: ReturnType<typeof createGraphFilterState>,
  graph: ActorGraphResponse,
  reset = false
) {
  const next = reset ? createGraphFilterState() : cloneGraphFilterState(current);
  syncGraphFilterStateWithResponse(next, graph, { reset });
  return next;
}

export function ActorGraphPage() {
  const queryClient = useQueryClient();
  const actorsQuery = useQuery({
    queryKey: ["actors"],
    queryFn: listActors,
  });
  const runsQuery = useQuery({
    queryKey: ["actor-graph-runs"],
    queryFn: listActorGraphRuns,
  });
  const annotationsQuery = useQuery({
    queryKey: ["annotations"],
    queryFn: listAnnotations,
  });
  const blocklistQuery = useQuery({
    queryKey: ["blocklist"],
    queryFn: listBlocklist,
  });

  const [selectedActorIDs, setSelectedActorIDs] = useState<number[]>([]);
  const [form, setForm] = useState<GraphFormState>(defaultFormState);
  const [graph, setGraph] = useState<ActorGraphResponse | null>(null);
  const [selection, setSelection] = useState<GraphSelection>(null);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [statusText, setStatusText] = useState("Select actors and build a graph.");
  const [lookupResult, setLookupResult] = useState<ActionLookupResponse | null>(null);
  const [lookupError, setLookupError] = useState("");
  const [graphFilters, setGraphFilters] = useState(createGraphFilterState);
  const [expandedActorIDs, setExpandedActorIDs] = useState<number[]>([]);
  const [expandedExternalChains, setExpandedExternalChains] = useState<string[]>([]);
  const [expandedHopSeeds, setExpandedHopSeeds] = useState<string[]>([]);
  const [graphResetKey, setGraphResetKey] = useState(0);

  const buildMutation = useMutation({
    mutationFn: buildActorGraph,
    onSuccess: async (response, request) => {
      setGraph(response);
      setSelection(null);
      setLookupResult(null);
      setLookupError("");
      setExpandedActorIDs([]);
      setExpandedExternalChains([]);
      setExpandedHopSeeds([]);
      setGraphFilters((current) => nextFilterState(current, response, true));
      setGraphResetKey((value) => value + 1);
      setStatusText(
        `Loaded ${response.nodes.length} nodes and ${response.edges.length} edges for ${
          actorNames(response.actors) || "the selected actors"
        }.`
      );
      setForm(stateFromRequest(request));
      setSelectedActorIDs(request.actor_ids);
      await queryClient.invalidateQueries({ queryKey: ["actor-graph-runs"] });
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Graph build failed.");
    },
  });

  const deleteRunMutation = useMutation({
    mutationFn: deleteActorGraphRun,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["actor-graph-runs"] });
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
    () =>
      graph
        ? deriveActorVisibleGraph(graph, graphFilters, graphMetadata, {
            expandedActorIDs,
            expandedExternalChains,
          })
        : null,
    [expandedActorIDs, expandedExternalChains, graph, graphFilters, graphMetadata]
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

  function toggleActor(actorID: number) {
    setSelectedActorIDs((current) => toggleNumber(current, actorID));
  }

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

  async function handleLoadRun() {
    if (!selectedRun) {
      return;
    }
    setStatusText("Rebuilding saved run...");
    await buildMutation.mutateAsync(selectedRun.request);
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

    const seeds = actorExpansionSeeds(node, graph);
    if (!seeds.length) {
      setStatusText("Selected node has no address context to expand.");
      return;
    }

    const nextSeedSet = [...new Set([...expandedHopSeeds, ...seeds.map((seed) => seed.encoded)])];
    if (nextSeedSet.length === expandedHopSeeds.length) {
      setStatusText("One-hop expansion already loaded for this node.");
      return;
    }

    setStatusText(`Expanding one hop from ${seeds.length} address(es)…`);
    try {
      const response = await expandActorGraph({
        actor_ids: graph.query.actor_ids,
        addresses: nextSeedSet,
        start_time: graph.query.start_time,
        end_time: graph.query.end_time,
        flow_types: graph.query.flow_types,
        min_usd: graph.query.min_usd,
        collapse_external: graph.query.collapse_external,
        display_mode: graph.query.display_mode,
      });
      setExpandedHopSeeds(nextSeedSet);
      setGraph((current) => {
        const merged = mergeActorGraphResponse(current, response);
        setGraphFilters((filters) => nextFilterState(filters, merged, false));
        return merged;
      });
      setStatusText(`Loaded one-hop expansion for ${nextSeedSet.length} address seed(s).`);
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Expansion failed.");
    }
  }

  async function handleRefreshAllLiveHoldings() {
    if (!graph) {
      return;
    }
    try {
      const response = await refreshLiveHoldings(graph.nodes);
      setGraph((current) => {
        if (!current) {
          return current;
        }
        return {
          ...current,
          nodes: applyNodeUpdates(current.nodes, response.nodes),
          warnings: Array.from(new Set([...current.warnings, ...response.warnings])),
        };
      });
      setStatusText(
        response.warnings.length
          ? `Live holdings refreshed. ${response.warnings.join(" · ")}`
          : `Live holdings refreshed at ${formatShortDateTime(response.refreshed_at)}.`
      );
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Live holdings refresh failed.");
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
          nodes: applyNodeUpdates(current.nodes, response.nodes),
          warnings: Array.from(new Set([...current.warnings, ...response.warnings])),
        };
      });
      setStatusText(
        response.warnings.length
          ? `Refreshed live value for ${rawNodes.length} node(s). ${response.warnings.join(" · ")}`
          : `Refreshed live value for ${rawNodes.length} node(s).`
      );
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
      setStatusText("No nodes currently show a live value of Unavailable.");
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
          nodes: applyNodeUpdates(current.nodes, response.nodes),
          warnings: Array.from(new Set([...current.warnings, ...response.warnings])),
        };
      });
      setStatusText(
        response.warnings.length
          ? `Checked ${rawNodes.length} unavailable node(s). ${response.warnings.join(" · ")}`
          : `Checked ${rawNodes.length} unavailable node(s).`
      );
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Unavailable live value check failed.");
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

  const actorOptions = [...(actorsQuery.data ?? [])].sort((left, right) => left.name.localeCompare(right.name));
  const filtersActive = graphFiltersAreActive(graphFilters);
  const visibleNodeCount = visibleGraph?.nodes.length ?? 0;
  const visibleEdgeCount = visibleGraph?.edges.length ?? 0;
  const totalNodeCount = Number(graph?.stats?.node_count || graph?.nodes.length || 0);
  const totalEdgeCount = Number(graph?.stats?.edge_count || graph?.edges.length || 0);
  const totalActionCount = Number(graph?.stats?.supporting_action_count || graph?.supporting_actions.length || 0);
  const showNodeFraction = filtersActive || visibleNodeCount !== totalNodeCount;
  const showEdgeFraction = filtersActive || visibleEdgeCount !== totalEdgeCount;
  const showActionFraction = filtersActive || filteredActions.length !== totalActionCount;

  return (
    <div className="page-grid graph-layout">
      <ActorGraphSidebar
        actors={actorOptions}
        selectedActorIDs={selectedActorIDs}
        onToggleActor={toggleActor}
        form={form}
        onFormChange={setForm}
        onBuild={() => buildMutation.mutate(requestFromState(form, selectedActorIDs))}
        isBuilding={buildMutation.isPending}
        canBuild={selectedActorIDs.length > 0}
        onRefreshLiveHoldings={() => {
          void handleRefreshAllLiveHoldings();
        }}
        canRefreshLiveHoldings={Boolean(graph)}
        isRefreshingLiveHoldings={false}
        statusText={statusText}
        runs={runsQuery.data ?? []}
        selectedRunID={selectedRunID}
        onSelectedRunIDChange={setSelectedRunID}
        onLoadRun={() => {
          void handleLoadRun();
        }}
        onDeleteRun={() => {
          void handleDeleteRun();
        }}
        isDeletingRun={deleteRunMutation.isPending}
        hasSelectedRun={Boolean(selectedRun)}
        isLoadingRuns={runsQuery.isLoading}
      />

      <div className="page-stack">
        <section className="panel page-panel graph-card-shell">
          <div className="panel-head graph-head">
            <div>
              <span className="eyebrow">Graph</span>
              <h2>{graph ? "Current Flow Graph" : "No Graph Loaded"}</h2>
              {graph ? (
                <p>
                  Click actor nodes to expand owned addresses. Click external clusters to expand one chain. Double-click to
                  expand one hop. Right-click nodes for more actions.
                </p>
              ) : null}
            </div>
            {graph ? (
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

          {graph ? (
            <>
              <div className="chip-list">
                <span className="meta-chip">{graph.actors.length} actors</span>
                <span className="meta-chip">{graph.query.max_hops} hops</span>
                <span className="meta-chip">
                  {formatDateTime(graph.query.start_time)} → {formatDateTime(graph.query.end_time)}
                </span>
                <span className="meta-chip">
                  {graph.query.coverage_satisfied ? "Full cache coverage" : "Partial cache coverage"}
                </span>
                <span className="meta-chip">{graph.query.blocks_scanned} blocks scanned</span>
                {expandedHopSeeds.length ? <span className="meta-chip">+{expandedHopSeeds.length} one-hop seeds</span> : null}
                {filtersActive ? <span className="meta-chip">Filters active</span> : null}
                <span className="meta-chip">{formatUSD(graph.query.min_usd)} min</span>
              </div>

              {graph.warnings.length ? (
                <div className="warning-list">
                  {graph.warnings.map((warning) => (
                    <span key={warning} className="warning-chip">
                      {warning}
                    </span>
                  ))}
                </div>
              ) : null}

              {visibleGraph && visibleGraph.nodes.length ? (
                <GraphCanvas
                  mode="actor"
                  nodes={visibleGraph.nodes}
                  edges={visibleGraph.edges}
                  selection={selection}
                  onSelectionChange={setSelection}
                  onNodePrimaryAction={(node) => {
                    if (node.kind === "actor" && node.actor_ids.length === 1) {
                      setExpandedActorIDs((current) => toggleNumber(current, node.actor_ids[0]));
                      return true;
                    }
                    if (node.kind === "external_cluster" && node.chain) {
                      setExpandedExternalChains((current) => toggleString(current, node.chain));
                      return true;
                    }
                    return false;
                  }}
                  onNodeDoubleActivate={(node) => {
                    void handleExpandNode(node);
                  }}
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
              actions={filteredActions}
              onLookup={(txID) => {
                lookupMutation.mutate(txID);
              }}
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
              emptyMessage="Select a graph node or edge to inspect labels, metrics, and transactions."
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
