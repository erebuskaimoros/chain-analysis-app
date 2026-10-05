import type {
  ActionLookupResponse,
  Actor,
  ActorGraphExpandRequest,
  ActorMonitor,
  ActorSnapshot,
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
  JobSnapshot,
  LiveHoldingsRefreshNode,
  LiveHoldingsRefreshResponse,
  Case,
  CaseItem,
  CaseItemKind,
  TraceRequest,
  TraceResponse,
  TraceRun,
} from "./types";
import { preferAtTimeValues } from "./atTimeValues";
import { JobCanceledError } from "./jobErrors";

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

const JOB_POLL_MS = 750;

export { JobCanceledError, isJobCanceled } from "./jobErrors";

export interface JobWatchOptions<P> {
  onProgress?: (job: JobSnapshot<unknown, P>) => void;
  onPartial?: (partial: P) => void;
  isCanceled?: () => boolean;
  pollMs?: number;
  // Fetch the (possibly large) partial result only when its summary counts
  // change, instead of on every poll.
  partialOnCountChange?: boolean;
}

export function getJob<T, P = unknown>(id: string, includePartial = false) {
  return fetchJSON<JobSnapshot<T, P>>(`/api/v1/jobs/${encodeURIComponent(id)}${includePartial ? "?partial=1" : ""}`);
}

export function cancelJob(id: string) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/jobs/${encodeURIComponent(id)}`, { method: "DELETE" });
}

// runJob starts a background job and polls it until it finishes, reporting
// progress and (when requested) partial results along the way.
export async function runJob<T, P = unknown>(path: string, payload: unknown, options: JobWatchOptions<P> = {}): Promise<T> {
  let job = await fetchJSON<JobSnapshot<T, P>>(path, { method: "POST", body: JSON.stringify(payload) });
  const wantPartial = Boolean(options.onPartial);
  let lastPartialCounts = "";
  while (job.status === "running") {
    if (options.isCanceled?.()) {
      void cancelJob(job.id).catch(() => undefined);
      throw new JobCanceledError();
    }
    options.onProgress?.(job);
    await new Promise((resolve) => setTimeout(resolve, options.pollMs ?? JOB_POLL_MS));
    if (options.isCanceled?.()) {
      void cancelJob(job.id).catch(() => undefined);
      throw new JobCanceledError();
    }
    if (wantPartial && options.partialOnCountChange) {
      job = await getJob<T, P>(job.id, false);
      const counts = JSON.stringify(job.partial_counts ?? {});
      if (job.status === "running" && job.partial_counts && counts !== lastPartialCounts) {
        lastPartialCounts = counts;
        const withPartial = await getJob<T, P>(job.id, true);
        if (withPartial.status === "running" && withPartial.partial != null) {
          options.onPartial?.(withPartial.partial);
        }
        job = withPartial;
      }
      continue;
    }
    job = await getJob<T, P>(job.id, wantPartial);
    if (wantPartial && job.partial != null) {
      options.onPartial?.(job.partial);
    }
  }
  if (job.status === "succeeded") {
    return job.result as T;
  }
  throw new Error(job.error || `Job ${job.status}.`);
}

// A graph build's partial result: what the server has projected so far.
export type ActorGraphPartial = Pick<ActorGraphResponse, "query" | "actors" | "warnings" | "nodes" | "edges">;

export async function buildActorGraph(
  payload: ActorGraphRequest,
  onProgress?: (job: JobSnapshot) => void,
  options: { isCanceled?: () => boolean; onPartial?: (partial: ActorGraphResponse) => void } = {}
) {
  return preferAtTimeValues(
    await runJob<ActorGraphResponse, ActorGraphPartial>("/api/v1/jobs/actor-graph", payload, {
      onProgress: onProgress as ((job: JobSnapshot<unknown, ActorGraphPartial>) => void) | undefined,
      isCanceled: options.isCanceled,
      partialOnCountChange: true,
      onPartial: options.onPartial
        ? (partial) =>
            options.onPartial?.(
              preferAtTimeValues({
                ...partial,
                warnings: partial.warnings ?? [],
                nodes: partial.nodes ?? [],
                edges: partial.edges ?? [],
                stats: {},
                supporting_actions: [],
              })
            )
        : undefined,
    })
  );
}

export async function expandActorGraph(payload: ActorGraphExpandRequest) {
  return preferAtTimeValues(await runJob<ActorGraphResponse>("/api/v1/jobs/actor-graph/expand", payload));
}

// refreshLiveHoldingsInBackground runs a server-side live-holdings job. The
// server retries transient failures and backs off from rate limits; every
// node ends in a final state. Partial updates arrive through onPartial.
export function refreshLiveHoldingsInBackground(
  nodes: FlowNode[],
  options: { force?: boolean; onPartial?: (partial: LiveHoldingsRefreshResponse) => void; isCanceled?: () => boolean } = {}
) {
  return runJob<LiveHoldingsRefreshResponse, LiveHoldingsRefreshResponse>(
    "/api/v1/jobs/live-holdings",
    { nodes: nodes.map(toLiveHoldingsRefreshNode), force: Boolean(options.force) },
    { onPartial: options.onPartial, isCanceled: options.isCanceled }
  );
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

export async function buildAddressExplorer(
  payload: AddressExplorerRequest,
  options: { onProgress?: (job: JobSnapshot) => void; isCanceled?: () => boolean } = {}
) {
  return preferAtTimeValues(
    await runJob<AddressExplorerResponse>("/api/v1/jobs/address-explorer", payload, {
      onProgress: options.onProgress,
      isCanceled: options.isCanceled,
    })
  );
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

export function getActorMonitor(actorID: number) {
  return fetchJSON<ActorMonitor>(`/api/v1/actors/${actorID}/monitor`);
}

export function setActorWatch(actorID: number, watch: boolean) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/actors/${actorID}/watch`, { method: "PUT", body: JSON.stringify({ watch }) });
}

export function markActorViewed(actorID: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/actors/${actorID}/viewed`, { method: "POST" });
}

// refreshActor records a new monitoring snapshot (flows since the last one and
// current holdings) as a background job.
export function refreshActor(actorID: number, onProgress?: (job: JobSnapshot) => void) {
  return runJob<ActorSnapshot>("/api/v1/jobs/actor-refresh", { actor_id: actorID }, { onProgress });
}

// startTrace follows funds from the request's seeds as a background job; the
// server saves each finished trace as a run.
export function startTrace(
  payload: TraceRequest,
  onProgress?: (job: JobSnapshot) => void,
  options: { isCanceled?: () => boolean } = {}
) {
  return runJob<TraceResponse>("/api/v1/jobs/trace", payload, { onProgress, isCanceled: options.isCanceled });
}

export function listTraceRuns() {
  return fetchJSON<TraceRun[]>("/api/v1/traces");
}

export function getTraceRun(id: number) {
  return fetchJSON<TraceRun>(`/api/v1/traces/${id}`);
}

export function deleteTraceRun(id: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/traces/${id}`, { method: "DELETE" });
}

export function listCases() {
  return fetchJSON<Case[]>("/api/v1/cases");
}

export function getCase(id: number) {
  return fetchJSON<Case>(`/api/v1/cases/${id}`);
}

export function createCase(title: string, notesMD = "") {
  return fetchJSON<Case>("/api/v1/cases", { method: "POST", body: JSON.stringify({ title, notes_md: notesMD }) });
}

export function updateCase(id: number, title: string, notesMD: string) {
  return fetchJSON<Case>(`/api/v1/cases/${id}`, { method: "PUT", body: JSON.stringify({ title, notes_md: notesMD }) });
}

export function deleteCase(id: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/cases/${id}`, { method: "DELETE" });
}

export function addCaseItem(caseID: number, kind: CaseItemKind, ref: string, note = "") {
  return fetchJSON<CaseItem>(`/api/v1/cases/${caseID}/items`, { method: "POST", body: JSON.stringify({ kind, ref, note }) });
}

export function deleteCaseItem(caseID: number, itemID: number) {
  return fetchJSON<{ ok: boolean }>(`/api/v1/cases/${caseID}/items/${itemID}`, { method: "DELETE" });
}

export function caseExportURL(caseID: number, format: "md" | "csv") {
  return `/api/v1/cases/${caseID}/export?format=${format}`;
}
