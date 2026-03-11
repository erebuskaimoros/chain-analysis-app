import type { Actor, ActorGraphResponse, AddressExplorerResponse, FlowEdge, FlowNode } from "../types";
import {
  booleanMetric,
  filterTransactionsByTime,
  graphChainsAllowed,
  graphItemChainSet,
  graphTxnTypeAllowed,
  hiddenAddressSet,
  labelAnnotationMap,
  mergeLiveHoldingsStatus,
  normalizeEdgeTransactions,
  numberMetric,
  stringMetric,
  summarizeTransactions,
} from "./internals";
import {
  defaultNodeColor,
  finalizeVisibleGraph,
  makeVisibleEdgeAdder,
  type VisibleEdgeAccumulator,
  type VisibleNodeAccumulator,
} from "./deriveShared";
import { type ActorGraphViewState, type GraphFilterState, type GraphMetadata, type VisibleGraph } from "./types";
import { nodeAddress } from "./internals";

export function deriveActorVisibleGraph(
  response: ActorGraphResponse,
  filterState: GraphFilterState,
  metadata: GraphMetadata,
  viewState: ActorGraphViewState
): VisibleGraph {
  const actorByIDMap = new Map(response.actors.map((actor) => [actor.id, actor]));
  const expandedActors = new Set(viewState.expandedActorIDs);
  const expandedExternalChains = new Set(viewState.expandedExternalChains.map((chain) => chain.toUpperCase()));
  const rawNodes = response.nodes;
  const rawEdges = response.edges;
  const rawNodeByID = new Map(rawNodes.map((node) => [node.id, node]));
  const incidentCounts = new Map<string, number>();
  const nodeUSD = new Map<string, number>();
  const filteredRawEdges: Array<FlowEdge & { chainSet: string[] }> = [];
  const labelAnnotations = labelAnnotationMap(metadata.annotations);
  const hiddenAddresses = hiddenAddressSet(metadata);

  rawEdges.forEach((rawEdge) => {
    if (String(rawEdge.action_class || "").trim().toLowerCase() === "ownership") {
      return;
    }
    const sourceNode = rawNodeByID.get(rawEdge.from);
    const targetNode = rawNodeByID.get(rawEdge.to);
    if (!sourceNode || !targetNode) {
      return;
    }
    const chainSet = graphItemChainSet(sourceNode, targetNode);
    if (!graphTxnTypeAllowed(rawEdge.action_class, rawEdge.action_key, rawEdge.action_label, filterState)) {
      return;
    }
    if (!graphChainsAllowed(chainSet, filterState)) {
      return;
    }
    const filteredTransactions = filterTransactionsByTime(normalizeEdgeTransactions(rawEdge), filterState);
    if (!filteredTransactions.length) {
      return;
    }
    const summary = summarizeTransactions(filteredTransactions);
    filteredRawEdges.push({
      ...rawEdge,
      chainSet,
      transactions: filteredTransactions,
      tx_ids: summary.tx_ids,
      heights: summary.heights,
      assets: summary.assets,
      usd_spot: summary.usd_spot,
    });
    [rawEdge.from, rawEdge.to].forEach((id) => {
      incidentCounts.set(id, (incidentCounts.get(id) || 0) + 1);
      nodeUSD.set(id, (nodeUSD.get(id) || 0) + Number(summary.usd_spot || 0));
    });
  });

  const visibleNodes = new Map<string, VisibleNodeAccumulator>();
  const visibleEdges = new Map<string, VisibleEdgeAccumulator>();

  function isHiddenAddress(node: FlowNode) {
    const address = nodeAddress(node).toLowerCase();
    return address ? hiddenAddresses.has(address) : false;
  }

  function lowSignalExternal(node: FlowNode) {
    if (!response.query.collapse_external) {
      return false;
    }
    if (node.kind !== "external_address" || node.actor_ids.length > 0) {
      return false;
    }
    if (expandedExternalChains.has(String(node.chain || "").trim().toUpperCase())) {
      return false;
    }
    if ((incidentCounts.get(node.id) || 0) > 1) {
      return false;
    }
    const threshold = Math.max(0, Number(response.query.min_usd || 0));
    return (nodeUSD.get(node.id) || 0) < threshold;
  }

  function mapNodeID(rawNode: FlowNode | undefined) {
    if (!rawNode || isHiddenAddress(rawNode)) {
      return null;
    }
    if (rawNode.kind === "actor_address" && !rawNode.shared && rawNode.actor_ids.length === 1) {
      const ownerID = rawNode.actor_ids[0];
      const ownerActor = actorByIDMap.get(ownerID);
      const ownerAddressCount = ownerActor?.addresses.length ?? 0;
      if (ownerAddressCount <= 1 && !expandedActors.has(ownerID)) {
        return `actor:${ownerID}`;
      }
      return rawNode.id;
    }
    if (lowSignalExternal(rawNode)) {
      return `external_cluster:${String(rawNode.chain || "UNKNOWN").trim().toUpperCase() || "UNKNOWN"}`;
    }
    return rawNode.id;
  }

  function ensureVisibleNode(rawNode: FlowNode, mappedID: string | null) {
    if (!mappedID) {
      return;
    }
    if (mappedID.startsWith("external_cluster:")) {
      const chain = mappedID.slice("external_cluster:".length) || "UNKNOWN";
      const existing = visibleNodes.get(mappedID) ?? {
        id: mappedID,
        label: `${chain} External Cluster`,
        kind: "external_cluster",
        chain,
        stage: "external",
        depth: rawNode.depth,
        actor_ids: [],
        shared: false,
        collapsed: true,
        raw_node_ids: [],
        metrics: {
          address_count: 0,
          usd_spot: 0,
          live_holdings_usd_spot: 0,
          live_holdings_available: false,
          live_holdings_status: "",
        },
      };
      if (!existing.raw_node_ids.includes(rawNode.id)) {
        existing.raw_node_ids.push(rawNode.id);
        existing.metrics.address_count = Number(existing.metrics.address_count || 0) + 1;
        existing.metrics.usd_spot = Number(existing.metrics.usd_spot || 0) + (nodeUSD.get(rawNode.id) || 0);
        existing.metrics.live_holdings_usd_spot =
          Number(existing.metrics.live_holdings_usd_spot || 0) +
          numberMetric(rawNode.metrics, "live_holdings_usd_spot");
        existing.metrics.live_holdings_available =
          Boolean(existing.metrics.live_holdings_available) ||
          booleanMetric(rawNode.metrics, "live_holdings_available");
        existing.metrics.live_holdings_status = mergeLiveHoldingsStatus(
          stringMetric(existing.metrics, "live_holdings_status"),
          stringMetric(rawNode.metrics, "live_holdings_status")
        );
        existing.depth = Math.min(existing.depth, rawNode.depth);
      }
      visibleNodes.set(mappedID, existing);
      return;
    }

    const collapsedActorID = mappedID.startsWith("actor:") ? Number(mappedID.slice("actor:".length)) : null;
    const collapsedActor =
      collapsedActorID !== null && Number.isFinite(collapsedActorID)
        ? actorByIDMap.get(collapsedActorID) ?? null
        : null;
    const actor = rawNode.kind === "actor" && rawNode.actor_ids.length === 1 ? actorByIDMap.get(rawNode.actor_ids[0]) : null;
    const ownerActor = rawNode.actor_ids.length === 1 ? actorByIDMap.get(rawNode.actor_ids[0]) : null;
    const existing = visibleNodes.get(mappedID) ?? {
      id: mappedID,
      label: collapsedActor?.name || rawNode.label,
      kind: collapsedActor ? "actor" : rawNode.kind,
      chain: rawNode.chain,
      stage: rawNode.stage,
      depth: rawNode.depth,
      actor_ids: [...rawNode.actor_ids],
      shared: Boolean(rawNode.shared),
      collapsed: Boolean(rawNode.collapsed),
      raw_node_ids: [],
      metrics: { ...(rawNode.metrics ?? {}), live_holdings_usd_spot: 0 },
      color: actor?.color || ownerActor?.color || collapsedActor?.color || "#83a8dc",
    };

    if (!existing.raw_node_ids.includes(rawNode.id)) {
      existing.raw_node_ids.push(rawNode.id);
      existing.depth = Math.min(existing.depth, rawNode.depth);
      existing.actor_ids = [...new Set(existing.actor_ids.concat(rawNode.actor_ids))];
      existing.shared = existing.shared || Boolean(rawNode.shared);
      existing.metrics.live_holdings_usd_spot =
        numberMetric(existing.metrics, "live_holdings_usd_spot") + numberMetric(rawNode.metrics, "live_holdings_usd_spot");
      existing.metrics.live_holdings_available =
        Boolean(existing.metrics.live_holdings_available) ||
        booleanMetric(rawNode.metrics, "live_holdings_available");
      existing.metrics.live_holdings_status = mergeLiveHoldingsStatus(
        stringMetric(existing.metrics, "live_holdings_status"),
        stringMetric(rawNode.metrics, "live_holdings_status")
      );
    }

    if (rawNode.kind === "actor" && actor?.color) {
      existing.color = actor.color;
    }

    visibleNodes.set(mappedID, existing);
  }

  const addVisibleEdge = makeVisibleEdgeAdder(rawNodeByID, visibleEdges, ensureVisibleNode, mapNodeID);

  filteredRawEdges.forEach((rawEdge) => {
    addVisibleEdge(rawEdge);
  });

  rawEdges.forEach((rawEdge) => {
    if (String(rawEdge.action_class || "").trim().toLowerCase() !== "ownership") {
      return;
    }
    const sourceNode = rawNodeByID.get(rawEdge.from);
    const targetNode = rawNodeByID.get(rawEdge.to);
    const from = mapNodeID(sourceNode);
    const to = mapNodeID(targetNode);
    if (!from || !to || from === to) {
      return;
    }
    if (!visibleNodes.has(from) || !visibleNodes.has(to)) {
      return;
    }
    addVisibleEdge({
      ...rawEdge,
      chainSet: graphItemChainSet(sourceNode, targetNode),
      transactions: normalizeEdgeTransactions(rawEdge),
    });
  });

  return finalizeVisibleGraph(visibleNodes, visibleEdges, actorByIDMap, labelAnnotations);
}

export function deriveExplorerVisibleGraph(
  response: AddressExplorerResponse,
  filterState: GraphFilterState,
  metadata: GraphMetadata
): VisibleGraph {
  const rawNodes = response.nodes;
  const rawEdges = response.edges;
  const rawNodeByID = new Map(rawNodes.map((node) => [node.id, node]));
  const filteredRawEdges: Array<FlowEdge & { chainSet: string[] }> = [];
  const labelAnnotations = labelAnnotationMap(metadata.annotations);
  const hiddenAddresses = hiddenAddressSet(metadata);

  function isHiddenAddress(node: FlowNode) {
    const address = nodeAddress(node).toLowerCase();
    return address ? hiddenAddresses.has(address) : false;
  }

  rawEdges.forEach((rawEdge) => {
    if (String(rawEdge.action_class || "").trim().toLowerCase() === "ownership") {
      return;
    }
    const sourceNode = rawNodeByID.get(rawEdge.from);
    const targetNode = rawNodeByID.get(rawEdge.to);
    if (!sourceNode || !targetNode) {
      return;
    }
    const chainSet = graphItemChainSet(sourceNode, targetNode);
    if (!graphTxnTypeAllowed(rawEdge.action_class, rawEdge.action_key, rawEdge.action_label, filterState)) {
      return;
    }
    if (!graphChainsAllowed(chainSet, filterState)) {
      return;
    }
    const filteredTransactions = filterTransactionsByTime(normalizeEdgeTransactions(rawEdge), filterState);
    if (!filteredTransactions.length) {
      return;
    }
    const summary = summarizeTransactions(filteredTransactions);
    filteredRawEdges.push({
      ...rawEdge,
      chainSet,
      transactions: filteredTransactions,
      tx_ids: summary.tx_ids,
      heights: summary.heights,
      assets: summary.assets,
      usd_spot: summary.usd_spot,
    });
  });

  const visibleNodes = new Map<string, VisibleNodeAccumulator>();
  const visibleEdges = new Map<string, VisibleEdgeAccumulator>();

  function mapNodeID(rawNode: FlowNode | undefined) {
    if (!rawNode || isHiddenAddress(rawNode)) {
      return null;
    }
    return rawNode.id;
  }

  function ensureVisibleNode(rawNode: FlowNode, mappedID: string | null) {
    if (!mappedID) {
      return;
    }
    const existing = visibleNodes.get(mappedID) ?? {
      id: mappedID,
      label: rawNode.label,
      kind: rawNode.kind,
      chain: rawNode.chain,
      stage: rawNode.stage,
      depth: rawNode.depth,
      actor_ids: [...rawNode.actor_ids],
      shared: Boolean(rawNode.shared),
      collapsed: Boolean(rawNode.collapsed),
      raw_node_ids: [],
      metrics: { ...(rawNode.metrics ?? {}), live_holdings_usd_spot: 0 },
      color: rawNode.kind === "explorer_target" ? "#e67e22" : defaultNodeColor(rawNode.kind),
    };
    if (!existing.raw_node_ids.includes(rawNode.id)) {
      existing.raw_node_ids.push(rawNode.id);
      existing.depth = Math.min(existing.depth, rawNode.depth);
      existing.metrics.live_holdings_usd_spot =
        numberMetric(existing.metrics, "live_holdings_usd_spot") + numberMetric(rawNode.metrics, "live_holdings_usd_spot");
      existing.metrics.live_holdings_available =
        Boolean(existing.metrics.live_holdings_available) ||
        booleanMetric(rawNode.metrics, "live_holdings_available");
      existing.metrics.live_holdings_status = mergeLiveHoldingsStatus(
        stringMetric(existing.metrics, "live_holdings_status"),
        stringMetric(rawNode.metrics, "live_holdings_status")
      );
    }
    visibleNodes.set(mappedID, existing);
  }

  const addVisibleEdge = makeVisibleEdgeAdder(rawNodeByID, visibleEdges, ensureVisibleNode, mapNodeID);

  filteredRawEdges.forEach((rawEdge) => {
    addVisibleEdge(rawEdge);
  });

  rawEdges.forEach((rawEdge) => {
    if (String(rawEdge.action_class || "").trim().toLowerCase() !== "ownership") {
      return;
    }
    const sourceNode = rawNodeByID.get(rawEdge.from);
    const targetNode = rawNodeByID.get(rawEdge.to);
    const from = mapNodeID(sourceNode);
    const to = mapNodeID(targetNode);
    if (!from || !to || from === to) {
      return;
    }
    if (!visibleNodes.has(from) || !visibleNodes.has(to)) {
      return;
    }
    addVisibleEdge({
      ...rawEdge,
      chainSet: graphItemChainSet(sourceNode, targetNode),
      transactions: normalizeEdgeTransactions(rawEdge),
    });
  });

  return finalizeVisibleGraph(visibleNodes, visibleEdges, new Map<number, Actor>(), labelAnnotations);
}
