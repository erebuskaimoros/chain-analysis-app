import type { Actor, ActorGraphResponse, AddressExplorerResponse, FlowEdge, FlowNode, SupportingAction } from "../types";
import {
  actionKey,
  cloneMergedEdge,
  edgeMergeKey,
  ExplorerMergeSource,
  mergeEdgeTransactions,
  nodeAddress,
  nodeMergeKey,
  normalizeEdgeTransactions,
  summarizeTransactions,
  uniqueNumbers,
  uniqueStrings,
} from "./internals";

export function mergeActorGraphResponse(current: ActorGraphResponse | null, incoming: ActorGraphResponse) {
  if (!current) {
    return incoming;
  }

  const actorMap = new Map<number, Actor>();
  current.actors.forEach((actor) => actorMap.set(actor.id, actor));
  incoming.actors.forEach((actor) => actorMap.set(actor.id, actor));

  const nodeMap = new Map<string, FlowNode>();
  const nodeAlias = new Map<string, string>();
  const nodeKeyToID = new Map<string, string>();

  function mergeNode(node: FlowNode) {
    const mergeKey = nodeMergeKey(node);
    const existingID = nodeKeyToID.get(mergeKey) || node.id;
    nodeAlias.set(node.id, existingID);
    const existing = nodeMap.get(existingID);
    if (!existing) {
      nodeKeyToID.set(mergeKey, existingID);
      nodeMap.set(existingID, {
        ...node,
        id: existingID,
        actor_ids: [...node.actor_ids],
        metrics: { ...(node.metrics ?? {}) },
      });
      return;
    }
    existing.actor_ids = uniqueNumbers(existing.actor_ids.concat(node.actor_ids));
    existing.shared = Boolean(existing.shared || node.shared);
    existing.collapsed = Boolean(existing.collapsed && node.collapsed);
    existing.depth = Math.min(Number(existing.depth || 0), Number(node.depth || 0));
    existing.metrics = { ...(existing.metrics ?? {}), ...(node.metrics ?? {}) };
  }

  current.nodes.forEach(mergeNode);
  incoming.nodes.forEach(mergeNode);

  const edgeMap = new Map<string, FlowEdge>();
  function addEdge(edge: FlowEdge) {
    const canonicalID = edgeMergeKey(edge, nodeAlias);
    const existing = edgeMap.get(canonicalID);
    if (!existing) {
      edgeMap.set(canonicalID, cloneMergedEdge(edge, canonicalID, nodeAlias));
      return;
    }
    existing.actor_ids = uniqueNumbers(existing.actor_ids.concat(edge.actor_ids));
    existing.confidence = Math.max(Number(existing.confidence || 0), Number(edge.confidence || 0));
    existing.action_key = existing.action_key || edge.action_key || edge.action_class;
    existing.action_label = existing.action_label || edge.action_label || edge.action_class;
    existing.action_domain = existing.action_domain || edge.action_domain || edge.action_class;
    existing.validator_address = existing.validator_address || edge.validator_address || "";
    existing.validator_label = existing.validator_label || edge.validator_label || "";
    existing.contract_type = existing.contract_type || edge.contract_type || "";
    existing.contract_protocol = existing.contract_protocol || edge.contract_protocol || "";
    existing.source_protocols = uniqueStrings([...(existing.source_protocols || []), ...(edge.source_protocols || [])]);
    existing.transactions = mergeEdgeTransactions(existing.transactions, normalizeEdgeTransactions(edge));
    const summary = summarizeTransactions(existing.transactions);
    existing.tx_ids = summary.tx_ids;
    existing.heights = summary.heights;
    existing.assets = summary.assets;
    existing.usd_spot = summary.usd_spot;
  }
  current.edges.forEach(addEdge);
  incoming.edges.forEach(addEdge);

  const nodes = Array.from(nodeMap.values());
  const edges = Array.from(edgeMap.values());
  const supporting_actions = mergeSupportingActions(
    [current.supporting_actions, incoming.supporting_actions],
    nodeAlias
  );

  return {
    ...current,
    query: {
      ...current.query,
      blocks_scanned: Number(current.query.blocks_scanned || 0) + Number(incoming.query.blocks_scanned || 0),
      coverage_satisfied:
        Boolean(current.query.coverage_satisfied) && Boolean(incoming.query.coverage_satisfied),
    },
    actors: Array.from(actorMap.values()),
    warnings: uniqueStrings(current.warnings.concat(incoming.warnings)),
    nodes,
    edges,
    supporting_actions,
    stats: {
      ...current.stats,
      ...incoming.stats,
      actor_count: actorMap.size,
      node_count: nodes.length,
      edge_count: edges.length,
      supporting_action_count: supporting_actions.length,
    },
  };
}

export function mergeAddressExplorerResponse(
  current: AddressExplorerResponse | null,
  incoming: AddressExplorerResponse
) {
  return mergeExplorerLikeResponse(current, incoming);
}

export function mergeExplorerExpansionResponse(
  current: AddressExplorerResponse | null,
  incoming: ActorGraphResponse
) {
  return mergeExplorerLikeResponse(current, incoming as unknown as ExplorerMergeSource);
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

function mergeExplorerLikeResponse(
  current: AddressExplorerResponse | null,
  incoming: ExplorerMergeSource
): AddressExplorerResponse {
  if (!current) {
    if ("mode" in incoming && "address" in incoming && "query" in incoming) {
      return incoming as AddressExplorerResponse;
    }
    throw new Error("Explorer merge requires a base response for expansion payloads.");
  }

  const nodeMap = new Map<string, FlowNode>();
  const nodeAlias = new Map<string, string>();
  const nodeKeyToID = new Map<string, string>();

  function explorerCanonicalKey(node: FlowNode) {
    const address = nodeAddress(node).toLowerCase();
    const chain = String(node.chain || "").trim().toUpperCase();
    if (!address || !chain) {
      return "";
    }
    return `explorer-address|${chain}|${address}`;
  }

  function mergeNode(node: FlowNode) {
    const mergeKey = nodeMergeKey(node);
    const canonicalKey = explorerCanonicalKey(node);
    const existingID = nodeKeyToID.get(mergeKey) || (canonicalKey ? nodeKeyToID.get(canonicalKey) : undefined) || node.id;
    nodeAlias.set(node.id, existingID);
    const existing = nodeMap.get(existingID);
    if (!existing) {
      nodeKeyToID.set(mergeKey, existingID);
      if (node.kind === "explorer_target" && canonicalKey) {
        nodeKeyToID.set(canonicalKey, existingID);
      }
      nodeMap.set(existingID, {
        ...node,
        id: existingID,
        actor_ids: [...node.actor_ids],
        metrics: { ...(node.metrics ?? {}) },
      });
      return;
    }
    nodeKeyToID.set(mergeKey, existingID);
    if (canonicalKey && existing.kind === "explorer_target") {
      nodeKeyToID.set(canonicalKey, existingID);
    }
    existing.actor_ids = uniqueNumbers(existing.actor_ids.concat(node.actor_ids));
    existing.shared = Boolean(existing.shared || node.shared);
    existing.collapsed = Boolean(existing.collapsed && node.collapsed);
    existing.depth = Math.min(Number(existing.depth || 0), Number(node.depth || 0));
    existing.metrics = { ...(existing.metrics ?? {}), ...(node.metrics ?? {}) };
  }

  current.nodes.forEach(mergeNode);
  incoming.nodes.forEach(mergeNode);

  const edgeMap = new Map<string, FlowEdge>();
  function addEdge(edge: FlowEdge) {
    const canonicalID = edgeMergeKey(edge, nodeAlias);
    const existing = edgeMap.get(canonicalID);
    if (!existing) {
      edgeMap.set(canonicalID, cloneMergedEdge(edge, canonicalID, nodeAlias));
      return;
    }
    existing.confidence = Math.max(Number(existing.confidence || 0), Number(edge.confidence || 0));
    existing.transactions = mergeEdgeTransactions(existing.transactions, normalizeEdgeTransactions(edge));
    existing.source_protocols = uniqueStrings([...(existing.source_protocols || []), ...(edge.source_protocols || [])]);
    const summary = summarizeTransactions(existing.transactions);
    existing.tx_ids = summary.tx_ids;
    existing.heights = summary.heights;
    existing.assets = summary.assets;
    existing.usd_spot = summary.usd_spot;
  }
  current.edges.forEach(addEdge);
  incoming.edges.forEach(addEdge);

  const nodes = Array.from(nodeMap.values());
  const edges = Array.from(edgeMap.values());
  const supporting_actions = mergeSupportingActions(
    [current.supporting_actions, incoming.supporting_actions],
    nodeAlias
  );

  const incomingQuery = incoming.query as Partial<AddressExplorerResponse["query"]> | undefined;

  return {
    ...current,
    mode: incoming.mode || current.mode,
    raw_address: incoming.raw_address || current.raw_address,
    address: incoming.address || current.address,
    query: {
      ...current.query,
      ...(incomingQuery ?? {}),
    },
    active_chains:
      Array.isArray(incoming.active_chains) && incoming.active_chains.length
        ? incoming.active_chains
        : current.active_chains,
    seed_summaries:
      Array.isArray(incoming.seed_summaries) && incoming.seed_summaries.length
        ? incoming.seed_summaries
        : current.seed_summaries,
    direction_required:
      typeof incoming.direction_required === "boolean"
        ? incoming.direction_required
        : current.direction_required,
    run_label: incoming.run_label || current.run_label,
    warnings: uniqueStrings(current.warnings.concat(incoming.warnings)),
    nodes,
    edges,
    supporting_actions,
    loaded_actions: Number(current.loaded_actions || 0) + Number(incoming.loaded_actions || 0),
    has_more: typeof incoming.has_more === "boolean" ? incoming.has_more : current.has_more,
    next_offset: Number.isFinite(Number(incoming.next_offset))
      ? Number(incoming.next_offset)
      : current.next_offset,
    total_estimate: Number.isFinite(Number(incoming.total_estimate))
      ? Number(incoming.total_estimate)
      : current.total_estimate,
    stats: {
      ...current.stats,
      ...incoming.stats,
      node_count: nodes.length,
      edge_count: edges.length,
      supporting_action_count: supporting_actions.length,
    },
  };
}

function mergeSupportingActions(
  sources: SupportingAction[][],
  nodeAlias: Map<string, string>
) {
  const actionMap = new Map<string, SupportingAction>();

  sources.flat().forEach((action) => {
    const canonical = canonicalizeSupportingAction(action, nodeAlias);
    const existing = actionMap.get(actionKey(canonical));
    if (!existing) {
      actionMap.set(actionKey(canonical), canonical);
      return;
    }

    existing.actor_ids = uniqueNumbers(existing.actor_ids.concat(canonical.actor_ids));
    existing.usd_spot = Math.max(Number(existing.usd_spot || 0), Number(canonical.usd_spot || 0));
    existing.height = Math.min(
      Number(existing.height || Number.MAX_SAFE_INTEGER),
      Number(canonical.height || Number.MAX_SAFE_INTEGER)
    );
    if (!Number.isFinite(existing.height) || existing.height === Number.MAX_SAFE_INTEGER) {
      existing.height = Number(canonical.height || 0);
    }
    if (!existing.time || (canonical.time && canonical.time < existing.time)) {
      existing.time = canonical.time || existing.time;
    }
    if (!existing.action_label) {
      existing.action_label = canonical.action_label;
    }
    if (!existing.action_domain) {
      existing.action_domain = canonical.action_domain;
    }
    if (!existing.validator_address) {
      existing.validator_address = canonical.validator_address;
    }
    if (!existing.validator_label) {
      existing.validator_label = canonical.validator_label;
    }
    if (!existing.contract_type) {
      existing.contract_type = canonical.contract_type;
    }
    if (!existing.contract_protocol) {
      existing.contract_protocol = canonical.contract_protocol;
    }
    if (!existing.primary_asset) {
      existing.primary_asset = canonical.primary_asset;
    }
    if (!existing.amount_raw) {
      existing.amount_raw = canonical.amount_raw;
    }
  });

  return Array.from(actionMap.values());
}

function canonicalizeSupportingAction(action: SupportingAction, nodeAlias: Map<string, string>): SupportingAction {
  return {
    ...action,
    from_node: nodeAlias.get(action.from_node) || action.from_node,
    to_node: nodeAlias.get(action.to_node) || action.to_node,
    actor_ids: [...action.actor_ids],
  };
}
