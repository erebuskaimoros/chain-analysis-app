import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  buildAddressExplorer,
  deleteAddressExplorerRun,
  expandActorGraph,
  listAddressExplorerRuns,
  lookupAction,
} from "../../../lib/api";
import { DEFAULT_DISPLAY_MODE, DEFAULT_FLOW_TYPES } from "../../../lib/constants";
import { deriveExplorerVisibleGraph, filterSupportingActions, mergeAddressExplorerResponse, mergeExplorerExpansionResponse, explorerExpansionSeeds, type GraphSelection } from "../../../lib/graph";
import type { ActionLookupResponse, AddressExplorerRequest, AddressExplorerResponse } from "../../../lib/types";
import { useGraphFilterState } from "../../shared/graph-hooks/useGraphFilterState";
import { useGraphMetadata } from "../../shared/graph-hooks/useGraphMetadata";
import { useSelectionGuard } from "../../shared/graph-hooks/useSelectionGuard";
import { useSharedGraphNodeActions } from "../../shared/graph-hooks/useSharedGraphNodeActions";

export interface ExplorerFormState {
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

export function useExplorerGraphController() {
  const queryClient = useQueryClient();
  const runsQuery = useQuery({
    queryKey: ["address-explorer-runs"],
    queryFn: listAddressExplorerRuns,
  });
  const { metadata } = useGraphMetadata();
  const {
    graphFilters,
    filtersActive,
    clearFilterState,
    syncWithGraph,
    toggleTxnType,
    toggleChain,
    updateDate,
    updateNumber,
    resetAllFilters,
    toggleOpen,
    close,
  } = useGraphFilterState();

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
      clearFilterState();
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
        syncWithGraph(next, request.offset === 0);
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

  const visibleGraph = useMemo(
    () => (graph ? deriveExplorerVisibleGraph(graph, graphFilters, metadata) : null),
    [graph, graphFilters, metadata]
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
    unavailableEmptyMessage: "No unavailable nodes.",
    onRefreshNodeSuccess: (rawNodeCount) => `Refreshed live value for ${rawNodeCount} node(s).`,
    onRefreshUnavailableSuccess: (requestedCount, response) =>
      `Checked ${requestedCount} unavailable node(s); refreshed ${response.nodes.length}.`,
  });

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

  async function onLoadRun() {
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
    const seeds = explorerExpansionSeeds(node, graph);
    await expandFromSeeds(seeds, true);
  }

  async function onExpandNodes(nodes: NonNullable<typeof visibleGraph>["nodes"]) {
    if (!graph) {
      return;
    }
    const seeds = Array.from(
      new Map(
        nodes.flatMap((node) => explorerExpansionSeeds(node, graph)).map((seed) => [seed.encoded, seed])
      ).values()
    );
    await expandFromSeeds(seeds, false);
  }

  async function expandFromSeeds(
    seeds: ReturnType<typeof explorerExpansionSeeds>,
    singular: boolean
  ) {
    if (!graph) {
      return;
    }
    if (!seeds.length) {
      setStatusText(singular ? "Selected node has no address context to expand." : "Selected nodes have no address context to expand.");
      return;
    }
    const nextSeedSet = [...new Set([...expandedHopSeeds, ...seeds.map((seed) => seed.encoded)])];
    if (nextSeedSet.length === expandedHopSeeds.length) {
      setStatusText(singular ? "Already expanded from this node." : "Already expanded from the selected nodes.");
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
        syncWithGraph(next, false);
        return next;
      });
      setStatusText(`Expanded from ${nextSeedSet.length} address seed(s).`);
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Edge expansion failed.");
    }
  }

  const currentGraph = graph;
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

  return {
    form,
    setForm,
    preview,
    currentGraph,
    requestPreview,
    loadGraph,
    statusText,
    isPreviewing: previewMutation.isPending,
    isLoadingGraph: graphMutation.isPending,
    runs: runsQuery.data ?? [],
    selectedRunID,
    setSelectedRunID,
    onLoadRun,
    onDeleteRun,
    isDeletingRun: deleteRunMutation.isPending,
    hasSelectedRun: Boolean(selectedRun),
    isLoadingRuns: runsQuery.isLoading,
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
    onExpandNode,
    onExpandNodes,
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
