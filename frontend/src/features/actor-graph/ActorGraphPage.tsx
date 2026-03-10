import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  buildActorGraph,
  deleteActorGraphRun,
  listActorGraphRuns,
  listActors,
  lookupAction,
  refreshLiveHoldings,
  expandActorGraph,
} from "../../lib/api";
import { encodeActorAddressSeed } from "../../lib/actors";
import { DEFAULT_DISPLAY_MODE, DEFAULT_FLOW_TYPES, defaultActorGraphWindow } from "../../lib/constants";
import { formatShortDateTime, formatUSD, toLocalInputValue } from "../../lib/format";
import {
  applyNodeUpdates,
  mergeActorGraphResponse,
  nodeAddress,
  type GraphSelection,
} from "../../lib/graph";
import type {
  ActionLookupResponse,
  Actor,
  ActorGraphRequest,
  ActorGraphResponse,
  FlowNode,
} from "../../lib/types";
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

function expansionSeeds(node: FlowNode, graph: ActorGraphResponse): string[] {
  const seeds = new Set<string>();
  const directAddress = nodeAddress(node);
  if (directAddress) {
    seeds.add(node.chain ? `${node.chain}|${directAddress}` : directAddress);
  }

  node.actor_ids.forEach((actorID) => {
    const actor = graph.actors.find((item) => item.id === actorID);
    actor?.addresses.forEach((address) => {
      const seed = encodeActorAddressSeed(address.address, address.chain_hint);
      if (seed) {
        seeds.add(seed);
      }
    });
  });

  return Array.from(seeds);
}

function graphWarnings(graph: ActorGraphResponse | null) {
  return graph?.warnings ?? [];
}

function actorNames(actors: Actor[]) {
  return actors.map((actor) => actor.name).join(", ");
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

  const [selectedActorIDs, setSelectedActorIDs] = useState<number[]>([]);
  const [form, setForm] = useState<GraphFormState>(defaultFormState);
  const [graph, setGraph] = useState<ActorGraphResponse | null>(null);
  const [selection, setSelection] = useState<GraphSelection>(null);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [statusText, setStatusText] = useState("Select actors and build a graph.");
  const [lookupResult, setLookupResult] = useState<ActionLookupResponse | null>(null);
  const [lookupError, setLookupError] = useState("");

  const buildMutation = useMutation({
    mutationFn: buildActorGraph,
    onSuccess: async (response, request) => {
      setGraph(response);
      setSelection(null);
      setLookupResult(null);
      setLookupError("");
      setStatusText(`Loaded ${response.nodes.length} nodes and ${response.edges.length} edges for ${actorNames(response.actors) || "the selected actors"}.`);
      setForm(stateFromRequest(request));
      setSelectedActorIDs(request.actor_ids);
      await queryClient.invalidateQueries({ queryKey: ["actor-graph-runs"] });
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Graph build failed.");
    },
  });

  const expandMutation = useMutation({
    mutationFn: expandActorGraph,
    onSuccess: (response) => {
      setGraph((current) => mergeActorGraphResponse(current, response));
      setStatusText(`Expanded graph with ${response.nodes.length} additional nodes in the response batch.`);
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Expansion failed.");
    },
  });

  const liveHoldingsMutation = useMutation({
    mutationFn: refreshLiveHoldings,
    onSuccess: (response) => {
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
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Live holdings refresh failed.");
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

  function toggleActor(actorID: number) {
    setSelectedActorIDs((current) =>
      current.includes(actorID) ? current.filter((value) => value !== actorID) : [...current, actorID]
    );
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

  async function handleExpandNode(node: FlowNode) {
    if (!graph) {
      return;
    }

    const addresses = expansionSeeds(node, graph);
    if (!addresses.length) {
      setStatusText("Selected node does not expose any addresses for one-hop expansion.");
      return;
    }

    setStatusText(`Expanding ${node.label} across ${addresses.length} address seed(s)...`);
    await expandMutation.mutateAsync({
      actor_ids: graph.query.actor_ids,
      addresses,
      start_time: graph.query.start_time,
      end_time: graph.query.end_time,
      flow_types: graph.query.flow_types,
      min_usd: graph.query.min_usd,
      collapse_external: graph.query.collapse_external,
      display_mode: graph.query.display_mode,
    });
  }

  const actorOptions = [...(actorsQuery.data ?? [])].sort((left, right) => left.name.localeCompare(right.name));

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
        onRefreshLiveHoldings={() => graph && liveHoldingsMutation.mutate(graph.nodes)}
        canRefreshLiveHoldings={Boolean(graph)}
        isRefreshingLiveHoldings={liveHoldingsMutation.isPending}
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
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Graph</span>
              <h2>{graph ? "Current Flow Graph" : "No Graph Loaded"}</h2>
            </div>
            {graph ? <span className="status-pill ok">{graph.nodes.length} nodes</span> : null}
          </div>
          {graph ? (
            <>
              <div className="chip-list">
                <span className="meta-chip">{graph.edges.length} edges</span>
                <span className="meta-chip">{graph.supporting_actions.length} actions</span>
                <span className="meta-chip">{graph.query.max_hops} hops</span>
                <span className="meta-chip">{formatUSD(graph.query.min_usd)} min</span>
              </div>
              {graphWarnings(graph).length ? (
                <div className="warning-list">
                  {graphWarnings(graph).map((warning) => (
                    <span key={warning} className="warning-chip">
                      {warning}
                    </span>
                  ))}
                </div>
              ) : null}
              <GraphCanvas
                nodes={graph.nodes}
                edges={graph.edges}
                selection={selection}
                onSelectionChange={setSelection}
                onNodeDoubleActivate={(node) => {
                  void handleExpandNode(node);
                }}
              />
              <p className="section-note">Double-click a node to expand one hop from its underlying address set.</p>
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
              actions={graph?.supporting_actions ?? []}
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
            <ActionLookupPanel
              result={lookupResult}
              isLoading={lookupMutation.isPending}
              error={lookupError}
            />
          </section>
        </div>
      </div>
    </div>
  );
}
