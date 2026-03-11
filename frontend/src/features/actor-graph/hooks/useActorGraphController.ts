import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  buildActorGraph,
  deleteActorGraphRun,
  expandActorGraph,
  listActorGraphRuns,
  listActors,
  lookupAction,
  refreshLiveHoldings,
} from "../../../lib/api";
import { DEFAULT_DISPLAY_MODE, DEFAULT_FLOW_TYPES, defaultActorGraphWindow } from "../../../lib/constants";
import { formatShortDateTime, toLocalInputValue } from "../../../lib/format";
import {
  actorExpansionSeeds,
  applyNodeUpdates,
  deriveActorVisibleGraph,
  filterSupportingActions,
  mergeActorGraphResponse,
  type GraphSelection,
} from "../../../lib/graph";
import type {
  ActionLookupResponse,
  Actor,
  ActorGraphRequest,
  ActorGraphResponse,
} from "../../../lib/types";
import { useGraphFilterState } from "../../shared/graph-hooks/useGraphFilterState";
import { useGraphMetadata } from "../../shared/graph-hooks/useGraphMetadata";
import { useSelectionGuard } from "../../shared/graph-hooks/useSelectionGuard";
import { useSharedGraphNodeActions } from "../../shared/graph-hooks/useSharedGraphNodeActions";
import type { GraphFormState } from "../ActorGraphSidebar";

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

export function useActorGraphController() {
  const queryClient = useQueryClient();
  const actorsQuery = useQuery({
    queryKey: ["actors"],
    queryFn: listActors,
  });
  const runsQuery = useQuery({
    queryKey: ["actor-graph-runs"],
    queryFn: listActorGraphRuns,
  });
  const { metadata } = useGraphMetadata();
  const {
    graphFilters,
    filtersActive,
    syncWithGraph,
    toggleTxnType,
    toggleChain,
    updateDate,
    updateNumber,
    resetAllFilters,
    toggleOpen,
    close,
  } = useGraphFilterState();

  const [selectedActorIDs, setSelectedActorIDs] = useState<number[]>([]);
  const [form, setForm] = useState<GraphFormState>(defaultFormState);
  const [graph, setGraph] = useState<ActorGraphResponse | null>(null);
  const [selection, setSelection] = useState<GraphSelection>(null);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [statusText, setStatusText] = useState("Select actors and build a graph.");
  const [lookupResult, setLookupResult] = useState<ActionLookupResponse | null>(null);
  const [lookupError, setLookupError] = useState("");
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
      syncWithGraph(response, true);
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

  const visibleGraph = useMemo(
    () =>
      graph
        ? deriveActorVisibleGraph(graph, graphFilters, metadata, {
            expandedActorIDs,
            expandedExternalChains,
          })
        : null,
    [expandedActorIDs, expandedExternalChains, graph, graphFilters, metadata]
  );

  const filteredActions = useMemo(
    () => (graph ? filterSupportingActions(graph.supporting_actions, graph, graphFilters) : []),
    [graph, graphFilters]
  );

  useSelectionGuard(selection, setSelection, visibleGraph);

  const sharedNodeActions = useSharedGraphNodeActions({
    graph,
    setGraph,
    setStatusText,
    queryClient,
    unavailableEmptyMessage: "No nodes currently show a live value of Unavailable.",
    onRefreshNodeSuccess: (rawNodeCount, response) =>
      response.warnings.length
        ? `Refreshed live value for ${rawNodeCount} node(s). ${response.warnings.join(" · ")}`
        : `Refreshed live value for ${rawNodeCount} node(s).`,
    onRefreshUnavailableSuccess: (requestedCount, response) =>
      response.warnings.length
        ? `Checked ${requestedCount} unavailable node(s). ${response.warnings.join(" · ")}`
        : `Checked ${requestedCount} unavailable node(s).`,
  });

  function toggleActor(actorID: number) {
    setSelectedActorIDs((current) => toggleNumber(current, actorID));
  }

  async function onBuild() {
    await buildMutation.mutateAsync(requestFromState(form, selectedActorIDs));
  }

  async function onLoadRun() {
    if (!selectedRun) {
      return;
    }
    setStatusText("Rebuilding saved run...");
    await buildMutation.mutateAsync(selectedRun.request);
  }

  async function onDeleteRun() {
    if (!selectedRun) {
      return;
    }
    await deleteRunMutation.mutateAsync(selectedRun.id);
  }

  async function onExpandNode(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
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
        syncWithGraph(merged, false);
        return merged;
      });
      setStatusText(`Loaded one-hop expansion for ${nextSeedSet.length} address seed(s).`);
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Expansion failed.");
    }
  }

  async function onRefreshAllLiveHoldings() {
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

  function onNodePrimaryAction(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (node.kind === "actor" && node.actor_ids.length === 1) {
      setExpandedActorIDs((current) => toggleNumber(current, node.actor_ids[0]));
      return true;
    }
    if (node.kind === "external_cluster" && node.chain) {
      setExpandedExternalChains((current) => toggleString(current, node.chain));
      return true;
    }
    return false;
  }

  const actorOptions = [...(actorsQuery.data ?? [])].sort((left, right) => left.name.localeCompare(right.name));
  const visibleNodeCount = visibleGraph?.nodes.length ?? 0;
  const visibleEdgeCount = visibleGraph?.edges.length ?? 0;
  const totalNodeCount = Number(graph?.stats?.node_count || graph?.nodes.length || 0);
  const totalEdgeCount = Number(graph?.stats?.edge_count || graph?.edges.length || 0);
  const totalActionCount = Number(graph?.stats?.supporting_action_count || graph?.supporting_actions.length || 0);
  const showNodeFraction = filtersActive || visibleNodeCount !== totalNodeCount;
  const showEdgeFraction = filtersActive || visibleEdgeCount !== totalEdgeCount;
  const showActionFraction = filtersActive || filteredActions.length !== totalActionCount;

  return {
    actorOptions,
    selectedActorIDs,
    toggleActor,
    form,
    setForm,
    onBuild,
    isBuilding: buildMutation.isPending,
    canBuild: selectedActorIDs.length > 0,
    onRefreshAllLiveHoldings,
    statusText,
    runs: runsQuery.data ?? [],
    selectedRunID,
    setSelectedRunID,
    onLoadRun,
    onDeleteRun,
    isDeletingRun: deleteRunMutation.isPending,
    hasSelectedRun: Boolean(selectedRun),
    isLoadingRuns: runsQuery.isLoading,
    graph,
    visibleGraph,
    filteredActions,
    selection,
    setSelection,
    graphResetKey,
    graphFilters,
    filtersActive,
    filterActions: {
      toggleTxnType,
      toggleChain,
      updateDate,
      updateNumber,
      resetAllFilters,
      toggleOpen,
      close,
    },
    expandedHopSeeds,
    onNodePrimaryAction,
    onExpandNode,
    nodeActions: sharedNodeActions,
    visibleNodeCount,
    visibleEdgeCount,
    totalNodeCount,
    totalEdgeCount,
    totalActionCount,
    showNodeFraction,
    showEdgeFraction,
    showActionFraction,
    lookupResult,
    lookupError,
    isLookupLoading: lookupMutation.isPending,
    onLookup: (txID: string) => lookupMutation.mutate(txID),
  };
}
