import type {
  ActionLookupResponse,
  Actor,
  ActorGraphBuildProgress,
  ActorGraphExpandRequest,
  ActorGraphRequest,
  ActorGraphResponse,
  ActorGraphRunsResponse,
  ActorListResponse,
  AddressExplorerRequest,
  AddressExplorerResponse,
  AddressExplorerRunsResponse,
  AnnotationListResponse,
  BlocklistResponse,
  FlowNode,
  GraphStateDetail,
  GraphStateListResponse,
  GraphStateSummary,
  HealthSnapshot,
  LiveHoldingsRefreshNode,
  LiveHoldingsRefreshResponse,
} from "./types";

export async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });

  const body = (await response.json().catch(() => ({}))) as Record<string, unknown>;
  if (!response.ok) {
    const message = typeof body.error === "string" ? body.error : `Request failed (${response.status})`;
    throw new Error(message);
  }

  return body as T;
}

export function getHealth() {
  return fetchJSON<HealthSnapshot>("/api/v1/health");
}

export async function listActors() {
  const response = await fetchJSON<ActorListResponse>("/api/v1/actors");
  return response.actors;
}

export function createActor(payload: Omit<Actor, "id" | "addresses" | "created_at" | "updated_at"> & { addresses: Array<{ address: string; chain_hint: string; label: string }> }) {
  return fetchJSON<Actor>("/api/v1/actors", { method: "POST", body: JSON.stringify(payload) });
}

export function updateActor(id: number, payload: Omit<Actor, "id" | "addresses" | "created_at" | "updated_at"> & { addresses: Array<{ address: string; chain_hint: string; label: string }> }) {
  return fetchJSON<Actor>(`/api/v1/actors/${id}`, { method: "PUT", body: JSON.stringify(payload) });
}

export function deleteActor(id: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/actors/${id}`, { method: "DELETE" });
}

export async function listAnnotations() {
  const response = await fetchJSON<AnnotationListResponse>("/api/v1/annotations");
  return response.annotations;
}

export function upsertAnnotation(payload: { address: string; kind: string; value: string }) {
  return fetchJSON<{ ok: boolean }>("/api/v1/annotations", { method: "PUT", body: JSON.stringify(payload) });
}

export function deleteAnnotation(payload: { address: string; kind: string }) {
  return fetchJSON<{ ok: boolean }>("/api/v1/annotations", { method: "DELETE", body: JSON.stringify(payload) });
}

export async function listBlocklist() {
  const response = await fetchJSON<BlocklistResponse>("/api/v1/blocklist");
  return response.addresses;
}

export function addToBlocklist(payload: { address: string; reason: string }) {
  return fetchJSON<{ ok: boolean }>("/api/v1/blocklist", { method: "POST", body: JSON.stringify(payload) });
}

export function removeFromBlocklist(address: string) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/blocklist/${encodeURIComponent(address)}`, { method: "DELETE" });
}

export function buildActorGraph(payload: ActorGraphRequest) {
  return fetchJSON<ActorGraphResponse>("/api/v1/analysis/actor-graph", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function expandActorGraph(payload: ActorGraphExpandRequest) {
  return fetchJSON<ActorGraphResponse>("/api/v1/analysis/actor-graph/expand", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function refreshLiveHoldings(nodes: FlowNode[]) {
  return fetchJSON<LiveHoldingsRefreshResponse>("/api/v1/analysis/actor-graph/live-holdings", {
    method: "POST",
    body: JSON.stringify({ nodes: nodes.map(toLiveHoldingsRefreshNode) }),
  });
}

export async function listActorGraphRuns() {
  const response = await fetchJSON<ActorGraphRunsResponse>("/api/v1/runs/actor-graph");
  return response.runs;
}

export function deleteActorGraphRun(id: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/runs/actor-graph/${id}`, { method: "DELETE" });
}

export function buildAddressExplorer(payload: AddressExplorerRequest) {
  return fetchJSON<AddressExplorerResponse>("/api/v1/analysis/address-explorer", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function listAddressExplorerRuns() {
  const response = await fetchJSON<AddressExplorerRunsResponse>("/api/v1/runs/address-explorer");
  return response.runs;
}

export function deleteAddressExplorerRun(id: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/runs/address-explorer/${id}`, { method: "DELETE" });
}

export function lookupAction(txID: string) {
  return fetchJSON<ActionLookupResponse>(`/api/v1/actions/${encodeURIComponent(txID)}`);
}

export function getActorGraphBuildProgress(token: string) {
  return fetchJSON<ActorGraphBuildProgress>(
    `/api/v1/analysis/actor-graph/progress/${encodeURIComponent(token)}`
  );
}

export async function listGraphStates(kind: string) {
  const response = await fetchJSON<GraphStateListResponse>(
    `/api/v1/graph-states?kind=${encodeURIComponent(kind)}`
  );
  return response.states ?? [];
}

export function createGraphState(payload: {
  kind: string;
  name: string;
  state: unknown;
  node_count: number;
  edge_count: number;
}) {
  return fetchJSON<GraphStateSummary>("/api/v1/graph-states", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function getGraphState(id: number) {
  return fetchJSON<GraphStateDetail>(`/api/v1/graph-states/${id}`);
}

export function deleteGraphState(id: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/graph-states/${id}`, { method: "DELETE" });
}

function toLiveHoldingsRefreshNode(node: FlowNode): LiveHoldingsRefreshNode {
  return {
    id: node.id,
    kind: node.kind,
    chain: node.chain,
    metrics: pickLiveHoldingsRefreshMetrics(node.metrics),
  };
}

function pickLiveHoldingsRefreshMetrics(metrics: FlowNode["metrics"]) {
  if (!metrics) {
    return null;
  }
  const out: Record<string, unknown> = {};
  for (const key of ["address", "pool", "source_protocol", "live_holdings_status"]) {
    if (key in metrics) {
      out[key] = metrics[key];
    }
  }
  return Object.keys(out).length ? out : null;
}
