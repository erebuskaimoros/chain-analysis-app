import type {
  Actor,
  ActorGraphResponse,
  AddressExplorerResponse,
  FlowEdge,
  FlowNode,
  SupportingAction,
} from "./types";

export type GraphSelection =
  | { kind: "node"; node: FlowNode }
  | { kind: "edge"; edge: FlowEdge }
  | null;

export function nodeAddress(node: FlowNode | null | undefined) {
  if (!node?.metrics) {
    return "";
  }
  const value = node.metrics.address;
  return typeof value === "string" ? value.trim() : "";
}

export function nodeDisplayColor(node: FlowNode) {
  if (node.kind === "actor") {
    const value = node.metrics?.color;
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  switch (node.kind) {
    case "actor":
      return "#2d7ff9";
    case "actor_address":
      return "#52a4ff";
    case "node":
      return "#2c7f5a";
    case "pool":
      return "#c0831c";
    case "contract_address":
      return "#ba4a52";
    case "explorer_target":
      return "#1d5e8d";
    default:
      return "#7a7f85";
  }
}

export function edgeDisplayColor(actionClass: string) {
  switch (actionClass) {
    case "ownership":
      return "#9aa3ad";
    case "swaps":
      return "#b54d4d";
    case "bonds":
      return "#1f7f65";
    case "liquidity":
      return "#8e6a18";
    default:
      return "#476173";
  }
}

export function edgeWidth(usdSpot: number) {
  if (!Number.isFinite(usdSpot) || usdSpot <= 0) {
    return 2;
  }
  if (usdSpot >= 1_000_000) {
    return 8;
  }
  if (usdSpot >= 100_000) {
    return 6;
  }
  if (usdSpot >= 10_000) {
    return 4;
  }
  return 3;
}

function mergeNodes(current: FlowNode[], incoming: FlowNode[]) {
  const byID = new Map(current.map((node) => [node.id, node]));
  incoming.forEach((node) => {
    const existing = byID.get(node.id);
    if (!existing) {
      byID.set(node.id, node);
      return;
    }
    byID.set(node.id, {
      ...existing,
      ...node,
      actor_ids: uniqueNumbers([...existing.actor_ids, ...node.actor_ids]),
      metrics: {
        ...(existing.metrics ?? {}),
        ...(node.metrics ?? {}),
      },
    });
  });
  return Array.from(byID.values());
}

function mergeEdges(current: FlowEdge[], incoming: FlowEdge[]) {
  const byID = new Map(current.map((edge) => [edge.id, edge]));
  incoming.forEach((edge) => {
    byID.set(edge.id, edge);
  });
  return Array.from(byID.values());
}

function mergeActions(current: SupportingAction[], incoming: SupportingAction[]) {
  const byKey = new Map<string, SupportingAction>();
  [...current, ...incoming].forEach((action) => {
    const key = [action.tx_id, action.action_key, action.from_node, action.to_node].join("|");
    byKey.set(key, action);
  });
  return Array.from(byKey.values()).sort((left, right) => left.time.localeCompare(right.time));
}

export function mergeActorGraphResponse(current: ActorGraphResponse | null, incoming: ActorGraphResponse) {
  if (!current) {
    return incoming;
  }
  const nodes = mergeNodes(current.nodes, incoming.nodes);
  const edges = mergeEdges(current.edges, incoming.edges);
  const supportingActions = mergeActions(current.supporting_actions, incoming.supporting_actions);

  return {
    ...current,
    actors: mergeActors(current.actors, incoming.actors),
    warnings: uniqueStrings([...current.warnings, ...incoming.warnings]),
    nodes,
    edges,
    supporting_actions: supportingActions,
    stats: {
      ...current.stats,
      ...incoming.stats,
      node_count: nodes.length,
      edge_count: edges.length,
      supporting_action_count: supportingActions.length,
    },
  };
}

export function mergeAddressExplorerResponse(
  current: AddressExplorerResponse | null,
  incoming: AddressExplorerResponse
) {
  if (!current) {
    return incoming;
  }

  const nodes = mergeNodes(current.nodes, incoming.nodes);
  const edges = mergeEdges(current.edges, incoming.edges);
  const supportingActions = mergeActions(current.supporting_actions, incoming.supporting_actions);

  return {
    ...incoming,
    warnings: uniqueStrings([...current.warnings, ...incoming.warnings]),
    active_chains: uniqueStrings([...current.active_chains, ...incoming.active_chains]),
    nodes,
    edges,
    supporting_actions: supportingActions,
    loaded_actions: current.loaded_actions + incoming.loaded_actions,
    stats: {
      ...current.stats,
      ...incoming.stats,
      node_count: nodes.length,
      edge_count: edges.length,
      supporting_action_count: supportingActions.length,
    },
  };
}

export function applyNodeUpdates(nodes: FlowNode[], updates: FlowNode[]) {
  const byID = new Map(updates.map((node) => [node.id, node]));
  return nodes.map((node) => {
    const update = byID.get(node.id);
    if (!update) {
      return node;
    }
    return {
      ...node,
      ...update,
      metrics: {
        ...(node.metrics ?? {}),
        ...(update.metrics ?? {}),
      },
    };
  });
}

function uniqueStrings(values: string[]) {
  return Array.from(new Set(values.filter(Boolean)));
}

function uniqueNumbers(values: number[]) {
  return Array.from(new Set(values.filter((value) => Number.isFinite(value))));
}

function mergeActors(current: Actor[], incoming: Actor[]) {
  const byID = new Map(current.map((actor) => [actor.id, actor]));
  incoming.forEach((actor) => {
    byID.set(actor.id, actor);
  });
  return Array.from(byID.values());
}
