import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  buildActorGraph,
  createGraphState,
  deleteActorGraphRun,
  deleteGraphState,
  expandActorGraph,
  getGraphState,
  listActorGraphRuns,
  listActors,
  listGraphStates,
  lookupAction,
  refreshLiveHoldingsInBackground,
} from "../../../lib/api";
import { isJobCanceled } from "../../../lib/jobErrors";
import { DEFAULT_DISPLAY_MODE, DEFAULT_FLOW_TYPES, defaultActorGraphWindow } from "../../../lib/constants";
import { buildGraphStateFilename, downloadJSON } from "../../../lib/download";
import { formatShortDateTime, toLocalInputValue } from "../../../lib/format";
import {
  isRecord,
  readJSONFile,
  readNumberArray,
  readSavedGraphCanvasState,
  readStringArray,
  restoreSavedGraphFilters,
  type SavedGraphCanvasState,
} from "../../../lib/graphState";
import {
  actorExpansionSeeds,
  applyLabelCategoryFilter,
  applyNodeUpdates,
  cloneGraphFilterState,
  deriveActorVisibleGraph,
  labelCategoriesOf,
  filterSupportingActions,
  mergeActorGraphResponse,
  refreshableLiveValueNodes,
  type GraphSelection,
} from "../../../lib/graph";
import type {
  ActionLookupResponse,
  Actor,
  ActorGraphRequest,
  ActorGraphResponse,
  GraphRun,
  GraphStateSummary,
  JobSnapshot,
  LiveHoldingsRefreshResponse,
} from "../../../lib/types";
import { useToast } from "../../../app/toast";
import { useGraphFilterState } from "../../shared/graph-hooks/useGraphFilterState";
import { useGraphMetadata } from "../../shared/graph-hooks/useGraphMetadata";
import { useSelectionGuard } from "../../shared/graph-hooks/useSelectionGuard";
import { useSharedGraphNodeActions } from "../../shared/graph-hooks/useSharedGraphNodeActions";
import type { GraphFormState } from "../ActorGraphSidebar";

export interface BuildProgress {
  stage: string;
  done: number;
  total: number;
  message: string;
  nodes: number;
  edges: number;
}

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
    include_unpriced: Boolean(form.include_unpriced),
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
    include_unpriced: Boolean(request.include_unpriced),
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

function requestFromGraphQuery(graph: ActorGraphResponse): ActorGraphRequest {
  return {
    actor_ids: [...graph.query.actor_ids],
    start_time: graph.query.start_time,
    end_time: graph.query.end_time,
    max_hops: graph.query.max_hops || 4,
    flow_types: [...graph.query.flow_types],
    min_usd: Number(graph.query.min_usd || 0),
    collapse_external: Boolean(graph.query.collapse_external),
    display_mode: graph.query.display_mode || DEFAULT_DISPLAY_MODE,
  };
}

function normalizeActorGraphRequest(value: unknown, graph: ActorGraphResponse): ActorGraphRequest {
  const fallback = requestFromGraphQuery(graph);
  if (!isRecord(value)) {
    return fallback;
  }

  const actorIDs = readNumberArray(value.actor_ids);
  const flowTypes = readStringArray(value.flow_types);
  const maxHops = typeof value.max_hops === "number" && Number.isFinite(value.max_hops) ? value.max_hops : fallback.max_hops;
  const minUSD = typeof value.min_usd === "number" && Number.isFinite(value.min_usd) ? value.min_usd : fallback.min_usd;

  return {
    actor_ids: actorIDs.length ? actorIDs : fallback.actor_ids,
    start_time: typeof value.start_time === "string" ? value.start_time : fallback.start_time,
    end_time: typeof value.end_time === "string" ? value.end_time : fallback.end_time,
    max_hops: maxHops,
    flow_types: flowTypes.length ? flowTypes : fallback.flow_types,
    min_usd: minUSD,
    collapse_external: typeof value.collapse_external === "boolean" ? value.collapse_external : fallback.collapse_external,
    display_mode: typeof value.display_mode === "string" ? value.display_mode : fallback.display_mode,
  };
}

function normalizeActorFormState(value: unknown, fallback: GraphFormState): GraphFormState {
  if (!isRecord(value)) {
    return fallback;
  }

  return {
    start_time: typeof value.start_time === "string" ? value.start_time : fallback.start_time,
    end_time: typeof value.end_time === "string" ? value.end_time : fallback.end_time,
    max_hops:
      typeof value.max_hops === "number" && Number.isFinite(value.max_hops) ? value.max_hops : fallback.max_hops,
    min_usd: typeof value.min_usd === "string" ? value.min_usd : fallback.min_usd,
  };
}

function buildProgressText(job: JobSnapshot) {
  const fraction = job.total > 0 ? ` ${Math.min(job.done, job.total)}/${job.total}` : "";
  const detail = job.message ? ` (${job.message})` : "";
  const sofar = job.partial_counts?.nodes ? `, ${job.partial_counts.nodes} nodes so far` : "";
  return `Building graph: ${job.stage || "starting"}${fraction}${detail}${sofar}…`;
}

export function useActorGraphController() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const actorsQuery = useQuery({
    queryKey: ["actors"],
    queryFn: listActors,
  });
  const runsQuery = useQuery({
    queryKey: ["actor-graph-runs"],
    queryFn: listActorGraphRuns,
  });
  const savedStatesQuery = useQuery({
    queryKey: ["graph-states", "actor-graph"],
    queryFn: () => listGraphStates("actor-graph"),
  });
  const { metadata } = useGraphMetadata();
  const {
    graphFilters,
    setGraphFilters,
    filtersActive,
    syncWithGraph,
    toggleTxnType,
    toggleChain,
    toggleLabelCategory,
    updateDate,
    updateNumber,
    resetAllFilters,
    toggleOpen,
    close,
  } = useGraphFilterState();

  const [selectedActorIDs, setSelectedActorIDs] = useState<number[]>([]);
  const [form, setForm] = useState<GraphFormState>(defaultFormState);
  const [graph, setGraph] = useState<ActorGraphResponse | null>(null);
  const [isPartialGraph, setIsPartialGraph] = useState(false);
  const [buildProgress, setBuildProgress] = useState<BuildProgress | null>(null);
  const [selection, setSelection] = useState<GraphSelection>(null);
  const [statusText, setStatusText] = useState("Pick one or more actors, then build the graph.");
  const [lookupResult, setLookupResult] = useState<ActionLookupResponse | null>(null);
  const [lookupError, setLookupError] = useState("");
  const [lookupTxID, setLookupTxID] = useState("");
  const [expandedActorIDs, setExpandedActorIDs] = useState<number[]>([]);
  const [expandedExternalChains, setExpandedExternalChains] = useState<string[]>([]);
  const [expandedHopSeeds, setExpandedHopSeeds] = useState<string[]>([]);
  const [isExpanding, setIsExpanding] = useState(false);
  const [graphResetKey, setGraphResetKey] = useState(0);
  const [savedCanvasState, setSavedCanvasState] = useState<SavedGraphCanvasState | null>(null);
  const [isRefreshingLiveHoldings, setIsRefreshingLiveHoldings] = useState(false);
  const liveRefreshRunRef = useRef(0);
  const buildRunRef = useRef(0);
  const graphBeforeBuildRef = useRef<ActorGraphResponse | null>(null);

  function mergeLiveHoldingsResponse(response: LiveHoldingsRefreshResponse) {
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
  }

  async function refreshGraphLiveHoldings(nodes: ActorGraphResponse["nodes"], mode: "auto" | "manual") {
    const refreshableNodes = refreshableLiveValueNodes(nodes);
    if (!refreshableNodes.length) {
      if (mode === "manual") {
        setStatusText("All graph live values are already computed inline.");
      }
      return;
    }

    const runID = liveRefreshRunRef.current + 1;
    liveRefreshRunRef.current = runID;
    setIsRefreshingLiveHoldings(true);
    const isStale = () => liveRefreshRunRef.current !== runID;

    try {
      // The server retries transient failures, waits out rate limits, and
      // returns every node in a final state; partial updates stream in.
      const response = await refreshLiveHoldingsInBackground(refreshableNodes, {
        force: mode === "manual",
        isCanceled: isStale,
        onPartial: (partial) => {
          if (!isStale()) {
            mergeLiveHoldingsResponse(partial);
          }
        },
      });
      if (isStale()) {
        return;
      }
      mergeLiveHoldingsResponse(response);
      if (response.warnings.length > 0) {
        const prefix = mode === "manual" ? "Live holdings refreshed." : "Background live holdings refresh finished.";
        setStatusText(`${prefix} ${response.warnings.join(" · ")}`);
        return;
      }
      if (mode === "manual" && response.refreshed_at) {
        setStatusText(`Live holdings refreshed at ${formatShortDateTime(response.refreshed_at)}.`);
      }
    } catch (error) {
      if (isStale()) {
        return;
      }
      setStatusText(error instanceof Error ? error.message : "Live holdings refresh failed.");
    } finally {
      if (liveRefreshRunRef.current === runID) {
        setIsRefreshingLiveHoldings(false);
      }
    }
  }

  const buildMutation = useMutation({
    mutationFn: (request: ActorGraphRequest) => {
      const runID = buildRunRef.current + 1;
      buildRunRef.current = runID;
      const isCanceled = () => buildRunRef.current !== runID;
      return buildActorGraph(
        request,
        (job) => {
          if (isCanceled()) {
            return;
          }
          setStatusText(buildProgressText(job));
          setBuildProgress({
            stage: job.stage || "starting",
            done: job.done,
            total: job.total,
            message: job.message ?? "",
            nodes: job.partial_counts?.nodes ?? 0,
            edges: job.partial_counts?.edges ?? 0,
          });
        },
        {
          isCanceled,
          onPartial: (partial) => {
            // Draw what the server has so far; the finished graph replaces it
            // with a full layout.
            if (isCanceled() || !partial.nodes.length) {
              return;
            }
            setGraph(partial);
            setIsPartialGraph(true);
            syncWithGraph(partial, true);
          },
        }
      );
    },
    onMutate: () => {
      graphBeforeBuildRef.current = graph && !isPartialGraph ? graph : graphBeforeBuildRef.current;
      setBuildProgress({ stage: "starting", done: 0, total: 0, message: "", nodes: 0, edges: 0 });
      setGraph(null);
      setIsPartialGraph(false);
      setSelection(null);
    },
    onSuccess: async (response, request) => {
      graphBeforeBuildRef.current = null;
      setBuildProgress(null);
      setIsPartialGraph(false);
      setGraph(response);
      setSelection(null);
      setLookupResult(null);
      setLookupError("");
      setLookupTxID("");
      setExpandedActorIDs([]);
      setExpandedExternalChains([]);
      setExpandedHopSeeds([]);
      setSavedCanvasState(null);
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
      void refreshGraphLiveHoldings(response.nodes, "auto");
    },
    onError: (error) => {
      setBuildProgress(null);
      const canceled = isJobCanceled(error);
      const previous = graphBeforeBuildRef.current;
      graphBeforeBuildRef.current = null;
      if (previous) {
        // Put the graph from before the build back rather than leave a
        // half-built one on screen.
        setGraph(previous);
        setIsPartialGraph(false);
        syncWithGraph(previous, true);
        setGraphResetKey((value) => value + 1);
      }
      setStatusText(
        canceled
          ? previous
            ? "Build canceled. Showing the graph from before."
            : "Build canceled."
          : error instanceof Error
            ? error.message
            : "Graph build failed."
      );
    },
  });

  const deleteRunMutation = useMutation({
    mutationFn: deleteActorGraphRun,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["actor-graph-runs"] });
    },
  });

  const deleteSavedStateMutation = useMutation({
    mutationFn: deleteGraphState,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["graph-states", "actor-graph"] });
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

  const visibleGraph = useMemo(
    () =>
      applyLabelCategoryFilter(
        graph
          ? deriveActorVisibleGraph(graph, graphFilters, metadata, {
              expandedActorIDs,
              expandedExternalChains,
            })
          : null,
        graphFilters.hiddenLabelCategories
      ),
    [expandedActorIDs, expandedExternalChains, graph, graphFilters, metadata]
  );
  const defaultSaveStateName = graph
    ? `${actorNames(graph.actors) || selectedActorIDs.join("-") || "graph"}, ${formatShortDateTime(new Date().toISOString())}`
    : "graph state";

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
    await buildWithProgress(requestFromState(form, selectedActorIDs));
  }

  // Builds run as server-side jobs; progress narrates long uncached scans.
  async function buildWithProgress(request: ActorGraphRequest) {
    setStatusText("Building graph…");
    try {
      await buildMutation.mutateAsync(request);
    } catch {
      // onError reports the failure in the status line.
    }
  }

  function cancelBuild() {
    if (!buildMutation.isPending) {
      return;
    }
    // The job poller sees the run change, cancels the server job and rejects.
    buildRunRef.current += 1;
    setStatusText("Canceling the build…");
  }

  // Saved runs store the request, not the graph, so opening one builds it
  // again. The form shows the run's settings straight away.
  async function onRunAgain(run: GraphRun) {
    setForm(stateFromRequest(run.request));
    setSelectedActorIDs([...run.request.actor_ids]);
    setStatusText(`Building ${run.actor_names || "the saved run"} again…`);
    await buildWithProgress(run.request);
  }

  async function onDeleteRun(run: GraphRun) {
    await deleteRunMutation.mutateAsync(run.id);
    toast(`Deleted the run for ${run.actor_names || "the selected actors"}`);
  }

  // Opening the page from a link or search: "Graph TC Treasury", a saved
  // graph from Home or a case, or a recent build to run again.
  function applyIntent(params: Record<string, string>) {
    if (params.state) {
      void onOpenSavedState({ id: Number(params.state) } as GraphStateSummary);
      return;
    }
    if (params.run_id) {
      void queryClient
        .fetchQuery({ queryKey: ["actor-graph-runs"], queryFn: listActorGraphRuns })
        .then((runs) => {
          const run = runs.find((item) => String(item.id) === params.run_id);
          if (run) {
            void onRunAgain(run);
          } else {
            setStatusText("That build is no longer in the recent builds list.");
          }
        });
      return;
    }
    const actorIDs = (params.actors ?? "")
      .split(",")
      .map((value) => Number(value))
      .filter((value) => Number.isFinite(value) && value > 0);
    if (!actorIDs.length) {
      return;
    }
    setSelectedActorIDs(actorIDs);
    if (params.run === "1") {
      void buildWithProgress(requestFromState(form, actorIDs));
    }
  }

  async function onExpandNode(node: NonNullable<typeof visibleGraph>["nodes"][number]) {
    if (!graph) {
      return;
    }
    const seeds = actorExpansionSeeds(node, graph);
    await expandFromSeeds(seeds, true, "edge");
  }

  async function onExpandNodes(nodes: NonNullable<typeof visibleGraph>["nodes"]) {
    if (!graph) {
      return;
    }
    const seeds = Array.from(
      new Map(
        nodes.flatMap((node) => actorExpansionSeeds(node, graph)).map((seed) => [seed.encoded, seed])
      ).values()
    );
    await expandFromSeeds(seeds, false, "edge");
  }

  async function expandFromSeeds(
    seeds: ReturnType<typeof actorExpansionSeeds>,
    singular: boolean,
    distanceLabel: "edge"
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
      setStatusText(
        singular
          ? `One-${distanceLabel} expansion already loaded for this node.`
          : `One-${distanceLabel} expansion already loaded for the selected nodes.`
      );
      return;
    }

    setStatusText(`Expanding one ${distanceLabel} from ${seeds.length} address(es)…`);
    setIsExpanding(true);
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
      setSavedCanvasState(null);
      setGraph((current) => {
        const merged = mergeActorGraphResponse(current, response);
        syncWithGraph(merged, false);
        return merged;
      });
      setStatusText(`Loaded one-${distanceLabel} expansion for ${nextSeedSet.length} address seed(s).`);
      void refreshGraphLiveHoldings(response.nodes, "auto");
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Expansion failed.");
    } finally {
      setIsExpanding(false);
    }
  }

  async function onRefreshAllLiveHoldings() {
    if (!graph) {
      return;
    }
    await refreshGraphLiveHoldings(graph.nodes, "manual");
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

  // The state payload persisted server-side and exported to files. Derived
  // data (visible graph, filtered actions) is intentionally omitted — it is
  // recomputed on load.
  function buildGraphStatePayload(canvasState: SavedGraphCanvasState | null) {
    if (!graph) {
      return null;
    }
    return {
      schema_version: 1,
      kind: "actor-graph",
      exported_at: new Date().toISOString(),
      request: requestFromState(form, selectedActorIDs),
      query: graph.query,
      ui_state: {
        selected_actor_ids: selectedActorIDs,
        form,
        filters: cloneGraphFilterState(graphFilters),
        selection,
        expanded_actor_ids: expandedActorIDs,
        expanded_external_chains: expandedExternalChains,
        expanded_hop_seeds: expandedHopSeeds,
        canvas: canvasState,
      },
      graph,
    };
  }

  async function onSaveGraphState(canvasState: SavedGraphCanvasState, name: string) {
    const payload = buildGraphStatePayload(canvasState);
    if (!payload || !graph) {
      return;
    }
    const finalName = name.trim() || defaultSaveStateName;
    try {
      const summary = await createGraphState({
        kind: "actor-graph",
        name: finalName,
        state: payload,
        node_count: graph.nodes.length,
        edge_count: graph.edges.length,
      });
      await queryClient.invalidateQueries({ queryKey: ["graph-states", "actor-graph"] });
      setStatusText(`Saved graph state "${summary.name}".`);
      toast(`Saved ${summary.name}`);
    } catch (error) {
      setStatusText(error instanceof Error ? `Could not save graph state: ${error.message}` : "Could not save graph state.");
    }
  }

  function applyGraphStatePayload(parsed: unknown, sourceLabel: string) {
    if (!isRecord(parsed)) {
      setStatusText(`Could not load ${sourceLabel}: invalid graph state payload.`);
      return false;
    }
    if (parsed.kind !== "actor-graph") {
      const foundKind = typeof parsed.kind === "string" ? parsed.kind : "unknown";
      setStatusText(`Could not load ${sourceLabel}: expected an actor-graph state, found ${foundKind}.`);
      return false;
    }
    const nextGraph = parsed.graph;
    if (!isRecord(nextGraph)) {
      setStatusText(`Could not load ${sourceLabel}: saved graph data is missing.`);
      return false;
    }

    const graphState = nextGraph as unknown as ActorGraphResponse;
    const savedRequest = normalizeActorGraphRequest(parsed.request, graphState);
    const uiState = isRecord(parsed.ui_state) ? parsed.ui_state : {};
    const nextSelectedActorIDs = readNumberArray(uiState.selected_actor_ids);

    setForm(normalizeActorFormState(uiState.form, stateFromRequest(savedRequest)));
    setSelectedActorIDs(nextSelectedActorIDs.length ? nextSelectedActorIDs : [...savedRequest.actor_ids]);
    setGraph(graphState);
    setIsPartialGraph(false);
    setSelection((uiState.selection as GraphSelection | null) ?? null);
    setLookupResult(null);
    setLookupError("");
    setLookupTxID("");
    setExpandedActorIDs(readNumberArray(uiState.expanded_actor_ids));
    setExpandedExternalChains(readStringArray(uiState.expanded_external_chains));
    setExpandedHopSeeds(readStringArray(uiState.expanded_hop_seeds));
    setSavedCanvasState(readSavedGraphCanvasState(uiState.canvas));
    setGraphFilters(restoreSavedGraphFilters(uiState.filters, graphState));
    setGraphResetKey((value) => value + 1);
    setStatusText(`Loaded graph state from ${sourceLabel}.`);
    void refreshGraphLiveHoldings(graphState.nodes, "auto");
    return true;
  }

  async function onLoadGraphState(file: File) {
    try {
      const parsed = await readJSONFile(file);
      applyGraphStatePayload(parsed, file.name);
    } catch (error) {
      setStatusText(error instanceof Error ? `Could not load ${file.name}: ${error.message}` : `Could not load ${file.name}.`);
    }
  }

  async function onOpenSavedState(state: GraphStateSummary) {
    try {
      const detail = await getGraphState(state.id);
      return applyGraphStatePayload(detail.state, `"${detail.name}"`);
    } catch (error) {
      setStatusText(error instanceof Error ? `Could not load saved state: ${error.message}` : "Could not load saved state.");
      return false;
    }
  }

  async function onDeleteSavedState(state: GraphStateSummary) {
    await deleteSavedStateMutation.mutateAsync(state.id);
    toast(`Deleted ${state.name}`);
  }

  async function onExportSavedState(state: GraphStateSummary) {
    try {
      const detail = await getGraphState(state.id);
      const filename = buildGraphStateFilename("actor-graph", detail.name);
      downloadJSON(filename, detail.state);
      setStatusText(`Exported "${detail.name}" to ${filename}.`);
    } catch (error) {
      setStatusText(error instanceof Error ? `Could not export saved state: ${error.message}` : "Could not export saved state.");
    }
  }

  function onLookup(txID: string) {
    setLookupTxID(txID);
    lookupMutation.mutate(txID);
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
    actorsLoading: actorsQuery.isLoading,
    selectedActorIDs,
    setSelectedActorIDs,
    toggleActor,
    form,
    setForm,
    onBuild,
    cancelBuild,
    isBuilding: buildMutation.isPending,
    buildProgress,
    isPartialGraph,
    isExpanding,
    canBuild: selectedActorIDs.length > 0,
    onRefreshAllLiveHoldings,
    isRefreshingLiveHoldings,
    statusText,
    runs: runsQuery.data ?? [],
    onRunAgain,
    onDeleteRun,
    isLoadingRuns: runsQuery.isLoading,
    savedStates: savedStatesQuery.data ?? [],
    onOpenSavedState,
    onDeleteSavedState,
    onExportSavedState,
    isLoadingSavedStates: savedStatesQuery.isLoading,
    applyIntent,
    graph,
    visibleGraph,
    filteredActions,
    selection,
    setSelection,
    graphResetKey,
    onSaveGraphState,
    defaultSaveStateName,
    onLoadGraphState,
    savedCanvasState,
    graphFilters,
    filtersActive,
    labelCategories: labelCategoriesOf(graph?.nodes),
    filterActions: {
      toggleTxnType,
      toggleChain,
      toggleLabelCategory,
      updateDate,
      updateNumber,
      resetAllFilters,
      toggleOpen,
      close,
    },
    expandedHopSeeds,
    onNodePrimaryAction,
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
    lookupTxID,
    isLookupLoading: lookupMutation.isPending,
    onLookup,
  };
}
