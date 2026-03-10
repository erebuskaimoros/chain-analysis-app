export interface BuildInfo {
  version: string;
  commit: string;
  build_time: string;
}

export interface HealthSnapshot {
  ok: boolean;
  time: string;
  build: BuildInfo;
  thornode_sources: string[];
  midgard_sources: string[];
  tracker_providers: Record<string, string>;
  tracker_overrides: Record<string, string>;
  tracker_candidates: Record<string, string[]>;
  tracker_health: Record<string, unknown>;
  tracker_sources: Record<string, unknown>;
}

export interface ActorAddressInput {
  address: string;
  chain_hint: string;
  label: string;
}

export interface ActorAddress extends ActorAddressInput {
  id: number;
  actor_id: number;
  normalized_address: string;
  created_at: string;
}

export interface Actor {
  id: number;
  name: string;
  color: string;
  notes: string;
  addresses: ActorAddress[];
  created_at: string;
  updated_at: string;
}

export interface ActorListResponse {
  actors: Actor[];
}

export interface AddressAnnotation {
  id: number;
  address: string;
  normalized_address: string;
  kind: string;
  value: string;
  created_at: string;
}

export interface AnnotationListResponse {
  annotations: AddressAnnotation[];
}

export interface BlocklistedAddress {
  id: number;
  address: string;
  normalized_address: string;
  reason: string;
  created_at: string;
}

export interface BlocklistResponse {
  addresses: BlocklistedAddress[];
}

export interface FlowAssetValue {
  asset: string;
  amount_raw: string;
  usd_spot: number;
  direction?: string;
  asset_kind?: string;
  token_standard?: string;
  token_address?: string;
  token_symbol?: string;
  token_name?: string;
  token_decimals?: number;
}

export interface FlowNode {
  id: string;
  kind: string;
  label: string;
  chain: string;
  stage: string;
  depth: number;
  actor_ids: number[];
  shared: boolean;
  collapsed: boolean;
  metrics: Record<string, unknown> | null;
}

export interface FlowEdgeTransaction {
  tx_id: string;
  height: number;
  time: string;
  usd_spot: number;
  assets: FlowAssetValue[];
}

export interface FlowEdge {
  id: string;
  from: string;
  to: string;
  action_class: string;
  action_key: string;
  action_label: string;
  action_domain: string;
  validator_address?: string;
  validator_label?: string;
  contract_type?: string;
  contract_protocol?: string;
  assets: FlowAssetValue[];
  transactions: FlowEdgeTransaction[];
  usd_spot: number;
  tx_ids: string[];
  heights: number[];
  actor_ids: number[];
  confidence: number;
}

export interface SupportingAction {
  tx_id: string;
  action_class: string;
  action_key: string;
  action_label: string;
  action_domain: string;
  validator_address?: string;
  validator_label?: string;
  contract_type?: string;
  contract_protocol?: string;
  primary_asset: string;
  amount_raw: string;
  usd_spot: number;
  height: number;
  time: string;
  from_node: string;
  to_node: string;
  actor_ids: number[];
}

export interface ActorTrackerQuery {
  actor_ids: number[];
  start_time: string;
  end_time: string;
  max_hops: number;
  flow_types: string[];
  min_usd: number;
  collapse_external: boolean;
  display_mode: string;
  requested_at: string;
  blocks_scanned: number;
  coverage_satisfied: boolean;
}

export interface ActorGraphRequest {
  actor_ids: number[];
  start_time: string;
  end_time: string;
  max_hops: number;
  flow_types: string[];
  min_usd: number;
  collapse_external: boolean;
  display_mode: string;
}

export interface ActorGraphExpandRequest {
  actor_ids: number[];
  addresses: string[];
  start_time: string;
  end_time: string;
  flow_types: string[];
  min_usd: number;
  collapse_external: boolean;
  display_mode: string;
}

export interface ActorGraphResponse {
  query: ActorTrackerQuery;
  actors: Actor[];
  stats: Record<string, number | string | boolean>;
  warnings: string[];
  nodes: FlowNode[];
  edges: FlowEdge[];
  supporting_actions: SupportingAction[];
}

export interface LiveHoldingsRefreshResponse {
  nodes: FlowNode[];
  warnings: string[];
  refreshed_at: string;
}

export interface AddressExplorerRequest {
  address: string;
  flow_types: string[];
  min_usd: number;
  mode: "preview" | "graph";
  direction: "" | "newest" | "oldest";
  offset: number;
  batch_size: number;
}

export interface AddressExplorerSeedSummary {
  chain: string;
  address: string;
  active: boolean;
  midgard_action_count: number;
  external_transfer_count: number;
}

export interface AddressExplorerQuery {
  address: string;
  flow_types: string[];
  min_usd: number;
  mode: string;
  direction: string;
  offset: number;
  batch_size: number;
}

export interface AddressExplorerResponse {
  mode: string;
  raw_address: string;
  address: string;
  query: AddressExplorerQuery;
  stats: Record<string, number | string | boolean>;
  warnings: string[];
  nodes: FlowNode[];
  edges: FlowEdge[];
  supporting_actions: SupportingAction[];
  loaded_actions: number;
  has_more: boolean;
  next_offset: number;
  total_estimate: number;
  direction_required: boolean;
  active_chains: string[];
  seed_summaries: AddressExplorerSeedSummary[];
  run_label: string;
}

export interface GraphRun {
  id: number;
  run_type?: string;
  request: ActorGraphRequest;
  actor_names: string;
  node_count: number;
  edge_count: number;
  created_at: string;
}

export interface ActorGraphRunsResponse {
  runs: GraphRun[];
}

export interface AddressExplorerRun {
  id: number;
  run_type?: string;
  request: AddressExplorerRequest;
  summary: string;
  node_count: number;
  edge_count: number;
  created_at: string;
}

export interface AddressExplorerRunsResponse {
  runs: AddressExplorerRun[];
}

export interface ActionLookupResponse {
  tx_id: string;
  actions: unknown[];
}
