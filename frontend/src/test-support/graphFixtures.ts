import type { GraphMetadata, VisibleGraphNode } from "../lib/graph";
import type {
  Actor,
  ActorGraphResponse,
  ActorAddress,
  AddressAnnotation,
  AddressExplorerResponse,
  BlocklistedAddress,
  FlowAssetValue,
  FlowEdge,
  FlowEdgeTransaction,
  FlowNode,
  SupportingAction,
} from "../lib/types";

type ActorGraphResponseOverrides = Partial<Omit<ActorGraphResponse, "query">> & {
  query?: Partial<ActorGraphResponse["query"]>;
};

type AddressExplorerResponseOverrides = Partial<Omit<AddressExplorerResponse, "query">> & {
  query?: Partial<AddressExplorerResponse["query"]>;
};

type FlowEdgeOverrides = Partial<Omit<FlowEdge, "transactions" | "assets" | "tx_ids" | "heights" | "usd_spot">> & {
  transactions?: FlowEdgeTransaction[];
  assets?: FlowAssetValue[];
  tx_ids?: string[];
  heights?: number[];
  usd_spot?: number;
};

export function makeAsset(overrides: Partial<FlowAssetValue> = {}): FlowAssetValue {
  return {
    asset: "THOR.RUNE",
    amount_raw: "100000000",
    usd_spot: 10,
    ...overrides,
  };
}

export function makeTransaction(overrides: Partial<FlowEdgeTransaction> = {}): FlowEdgeTransaction {
  const assets = (overrides.assets ?? [makeAsset({ usd_spot: Number(overrides.usd_spot ?? 10) })]).map((asset) => ({
    ...asset,
  }));
  return {
    tx_id: "tx-1",
    height: 1,
    time: "2026-01-01T00:00:00Z",
    usd_spot: 10,
    ...overrides,
    assets,
  };
}

export function makeNode(overrides: Partial<FlowNode> = {}): FlowNode {
  const metrics =
    overrides.metrics === undefined
      ? {
          address: `addr-${String(overrides.id ?? "node-1").toLowerCase()}`,
          live_holdings_available: false,
          live_holdings_status: "",
          live_holdings_usd_spot: 0,
        }
      : overrides.metrics === null
      ? null
      : {
          ...overrides.metrics,
        };
  const base: FlowNode = {
    id: "node-1",
    kind: "external_address",
    label: "Node 1",
    chain: "THOR",
    stage: "graph",
    depth: 0,
    actor_ids: [],
    shared: false,
    collapsed: false,
    metrics,
  };

  return {
    ...base,
    ...overrides,
    actor_ids: overrides.actor_ids ? [...overrides.actor_ids] : [],
    metrics,
  };
}

export function makeEdge(overrides: FlowEdgeOverrides = {}): FlowEdge {
  const transactions = (overrides.transactions ?? [makeTransaction({ tx_id: `${overrides.id ?? "edge-1"}-tx` })]).map(
    (transaction) => ({
      ...transaction,
      assets: transaction.assets.map((asset) => ({ ...asset })),
    })
  );
  const assets =
    overrides.assets?.map((asset) => ({ ...asset })) ??
    transactions.flatMap((transaction) => transaction.assets.map((asset) => ({ ...asset })));
  const txIDs = overrides.tx_ids ?? transactions.map((transaction) => transaction.tx_id);
  const heights = overrides.heights ?? transactions.map((transaction) => transaction.height);
  const usdSpot =
    overrides.usd_spot ?? transactions.reduce((sum, transaction) => sum + Number(transaction.usd_spot || 0), 0);

  const base: FlowEdge = {
    id: "edge-1",
    from: "node-1",
    to: "node-2",
    action_class: "transfers",
    action_key: "transfer",
    action_label: "Transfer",
    action_domain: "transfer",
    assets,
    transactions,
    usd_spot: usdSpot,
    tx_ids: txIDs,
    heights,
    actor_ids: [],
    confidence: 1,
  };

  return {
    ...base,
    ...overrides,
    assets,
    transactions,
    tx_ids: txIDs,
    heights,
    usd_spot: usdSpot,
    actor_ids: overrides.actor_ids ? [...overrides.actor_ids] : [],
  };
}

export function makeSupportingAction(overrides: Partial<SupportingAction> = {}): SupportingAction {
  const base: SupportingAction = {
    tx_id: "supporting-tx-1",
    action_class: "transfers",
    action_key: "transfer",
    action_label: "Transfer",
    action_domain: "transfer",
    primary_asset: "THOR.RUNE",
    amount_raw: "100000000",
    usd_spot: 10,
    height: 1,
    time: "2026-01-01T00:00:00Z",
    from_node: "node-1",
    to_node: "node-2",
    actor_ids: [],
  };

  return {
    ...base,
    ...overrides,
    actor_ids: overrides.actor_ids ? [...overrides.actor_ids] : [],
  };
}

export function makeActorAddress(overrides: Partial<ActorAddress> = {}): ActorAddress {
  return {
    id: 1,
    actor_id: 1,
    address: "thor1actor",
    chain_hint: "THOR",
    label: "Primary",
    normalized_address: "thor1actor",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export function makeActor(overrides: Partial<Actor> = {}): Actor {
  const addresses = (overrides.addresses ?? [makeActorAddress({ actor_id: overrides.id ?? 1 })]).map((address) => ({
    ...address,
  }));
  const base: Actor = {
    id: 1,
    name: "Actor 1",
    color: "#336699",
    notes: "",
    addresses,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };

  return {
    ...base,
    ...overrides,
    addresses,
  };
}

export function makeActorGraphResponse(overrides: ActorGraphResponseOverrides = {}): ActorGraphResponse {
  const actors = (overrides.actors ?? []).map((actor) => ({ ...actor, addresses: actor.addresses.map((address) => ({ ...address })) }));
  const nodes = (overrides.nodes ?? []).map((node) => ({ ...node, actor_ids: [...node.actor_ids] }));
  const edges = (overrides.edges ?? []).map((edge) => ({
    ...edge,
    actor_ids: [...edge.actor_ids],
    assets: edge.assets.map((asset) => ({ ...asset })),
    transactions: edge.transactions.map((transaction) => ({
      ...transaction,
      assets: transaction.assets.map((asset) => ({ ...asset })),
    })),
  }));
  const supportingActions = (overrides.supporting_actions ?? []).map((action) => ({
    ...action,
    actor_ids: [...action.actor_ids],
  }));

  return {
    query: {
      actor_ids: [],
      start_time: "2026-01-01T00:00:00Z",
      end_time: "2026-01-10T00:00:00Z",
      max_hops: 4,
      flow_types: ["transfers", "swaps", "bonds"],
      min_usd: 0,
      collapse_external: false,
      display_mode: "graph",
      requested_at: "2026-01-10T00:00:00Z",
      blocks_scanned: 10,
      coverage_satisfied: true,
      ...(overrides.query ?? {}),
    },
    actors,
    stats: {
      actor_count: actors.length,
      node_count: nodes.length,
      edge_count: edges.length,
      supporting_action_count: supportingActions.length,
      ...(overrides.stats ?? {}),
    },
    warnings: [...(overrides.warnings ?? [])],
    nodes,
    edges,
    supporting_actions: supportingActions,
  };
}

export function makeExplorerResponse(overrides: AddressExplorerResponseOverrides = {}): AddressExplorerResponse {
  const nodes = (overrides.nodes ?? []).map((node) => ({ ...node, actor_ids: [...node.actor_ids] }));
  const edges = (overrides.edges ?? []).map((edge) => ({
    ...edge,
    actor_ids: [...edge.actor_ids],
    assets: edge.assets.map((asset) => ({ ...asset })),
    transactions: edge.transactions.map((transaction) => ({
      ...transaction,
      assets: transaction.assets.map((asset) => ({ ...asset })),
    })),
  }));
  const supportingActions = (overrides.supporting_actions ?? []).map((action) => ({
    ...action,
    actor_ids: [...action.actor_ids],
  }));

  return {
    mode: overrides.mode ?? "graph",
    raw_address: overrides.raw_address ?? "thor1seed",
    address: overrides.address ?? "thor1seed",
    query: {
      address: overrides.address ?? "thor1seed",
      flow_types: ["transfers", "swaps", "bonds"],
      min_usd: 0,
      mode: overrides.mode ?? "graph",
      direction: "newest",
      offset: 0,
      batch_size: 10,
      ...(overrides.query ?? {}),
    },
    stats: {
      node_count: nodes.length,
      edge_count: edges.length,
      supporting_action_count: supportingActions.length,
      ...(overrides.stats ?? {}),
    },
    warnings: [...(overrides.warnings ?? [])],
    nodes,
    edges,
    supporting_actions: supportingActions,
    loaded_actions: overrides.loaded_actions ?? supportingActions.length,
    has_more: overrides.has_more ?? false,
    next_offset: overrides.next_offset ?? supportingActions.length,
    total_estimate: overrides.total_estimate ?? supportingActions.length,
    direction_required: overrides.direction_required ?? false,
    active_chains: [...(overrides.active_chains ?? [])],
    seed_summaries: [...(overrides.seed_summaries ?? [])],
    run_label: overrides.run_label ?? "Explorer Run",
  };
}

export function makeAnnotation(overrides: Partial<AddressAnnotation> = {}): AddressAnnotation {
  return {
    id: 1,
    address: "thor1labelled",
    normalized_address: "thor1labelled",
    kind: "label",
    value: "Custom Label",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export function makeBlocklistedAddress(overrides: Partial<BlocklistedAddress> = {}): BlocklistedAddress {
  return {
    id: 1,
    address: "thor1blocked",
    normalized_address: "thor1blocked",
    reason: "Hidden",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export function makeMetadata(overrides: Partial<GraphMetadata> = {}): GraphMetadata {
  return {
    annotations: [...(overrides.annotations ?? [])],
    blocklist: [...(overrides.blocklist ?? [])],
  };
}

export function makeVisibleNode(overrides: Partial<VisibleGraphNode> = {}): VisibleGraphNode {
  const id = overrides.id ?? "visible-node-1";
  const label = overrides.label ?? "Visible Node 1";
  const metrics =
    overrides.metrics === undefined
      ? {
          address: `addr-${String(id).toLowerCase()}`,
          live_holdings_available: false,
          live_holdings_status: "",
          live_holdings_usd_spot: 0,
        }
      : overrides.metrics === null
      ? null
      : {
          ...overrides.metrics,
        };

  return {
    id,
    label,
    displayLabel: overrides.displayLabel ?? label,
    kind: overrides.kind ?? "external_address",
    chain: overrides.chain ?? "THOR",
    stage: overrides.stage ?? "graph",
    depth: overrides.depth ?? 0,
    actor_ids: overrides.actor_ids ? [...overrides.actor_ids] : [],
    shared: overrides.shared ?? false,
    collapsed: overrides.collapsed ?? false,
    metrics,
    raw_node_ids: overrides.raw_node_ids ? [...overrides.raw_node_ids] : [id],
    live_holdings_label: overrides.live_holdings_label ?? "",
    live_holdings_status: overrides.live_holdings_status ?? "",
    color: overrides.color ?? "#5f86be",
    borderColor: overrides.borderColor ?? "#a2c4ff",
    chainLogo: overrides.chainLogo ?? "none",
    inspect: overrides.inspect ?? {},
    pie1Color: overrides.pie1Color,
    pie1Size: overrides.pie1Size,
    pie2Color: overrides.pie2Color,
    pie2Size: overrides.pie2Size,
    pie3Color: overrides.pie3Color,
    pie3Size: overrides.pie3Size,
    pie4Color: overrides.pie4Color,
    pie4Size: overrides.pie4Size,
  };
}
