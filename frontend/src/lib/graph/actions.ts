import type { ActorGraphResponse, AddressExplorerResponse, FlowNode } from "../types";
import { buildFrontierSeed, nodeAddress, uniqueSeeds, uniqueStrings, stringMetric } from "./internals";
import type { VisibleGraphNode } from "./types";

export { nodeAddress };

export function rawNodesForVisibleNode(
  node: Pick<VisibleGraphNode, "raw_node_ids" | "id">,
  graph: Pick<ActorGraphResponse | AddressExplorerResponse, "nodes">
) {
  const rawNodeByID = new Map(graph.nodes.map((rawNode) => [rawNode.id, rawNode]));
  const requestedIDs = node.raw_node_ids.length ? node.raw_node_ids : [node.id];
  const seen = new Set<string>();
  const out: FlowNode[] = [];

  requestedIDs.forEach((rawID) => {
    const rawNode = rawNodeByID.get(rawID);
    if (!rawNode || seen.has(rawID)) {
      return;
    }
    seen.add(rawID);
    out.push(rawNode);
  });

  return out;
}

export function unavailableRawNodes(graph: Pick<ActorGraphResponse | AddressExplorerResponse, "nodes"> | null) {
  if (!graph) {
    return [];
  }
  const unavailableStatuses = new Set(["error", "unavailable"]);
  const seen = new Set<string>();
  const out: FlowNode[] = [];

  graph.nodes.forEach((node) => {
    const nodeID = String(node.id || "").trim();
    const status = stringMetric(node.metrics, "live_holdings_status").toLowerCase();
    if (!nodeID || seen.has(nodeID) || !unavailableStatuses.has(status)) {
      return;
    }
    seen.add(nodeID);
    out.push(node);
  });

  return out;
}

export function actorExpansionSeeds(node: VisibleGraphNode, graph: ActorGraphResponse) {
  const rawNodeByID = new Map(graph.nodes.map((rawNode) => [rawNode.id, rawNode]));
  const out: Array<{ address: string; chain: string; encoded: string }> = [];

  node.raw_node_ids.forEach((rawID) => {
    const rawNode = rawNodeByID.get(rawID);
    const candidate = nodeAddress(rawNode);
    if (candidate) {
      const seed = buildFrontierSeed(candidate, rawNode?.chain);
      if (seed) {
        out.push(seed);
      }
    }
  });

  if (!out.length && node.kind === "actor") {
    node.actor_ids.forEach((actorID) => {
      const actor = graph.actors.find((item) => item.id === actorID);
      actor?.addresses.forEach((entry) => {
        const seed = buildFrontierSeed(entry.address, entry.chain_hint);
        if (seed) {
          out.push(seed);
        }
      });
    });
  }

  return uniqueSeeds(out);
}

export function explorerExpansionSeeds(node: VisibleGraphNode, graph: AddressExplorerResponse) {
  const rawNodeByID = new Map(graph.nodes.map((rawNode) => [rawNode.id, rawNode]));
  const out: Array<{ address: string; chain: string; encoded: string }> = [];

  node.raw_node_ids.forEach((rawID) => {
    const rawNode = rawNodeByID.get(rawID);
    const candidate = nodeAddress(rawNode);
    if (candidate) {
      const seed = buildFrontierSeed(candidate, rawNode?.chain);
      if (seed) {
        out.push(seed);
      }
    }
  });

  return uniqueSeeds(out);
}

export function nodeAddressForActions(
  node: VisibleGraphNode,
  graph: ActorGraphResponse | AddressExplorerResponse
) {
  const rawNodes = rawNodesForVisibleNode(node, graph);
  const addresses = rawNodes.map((rawNode) => nodeAddress(rawNode)).filter(Boolean);
  const unique = uniqueStrings(addresses);
  if (unique.length === 1) {
    return unique[0];
  }
  return "";
}

export function explorerURLForAddress(address: string, chain: string) {
  const rawAddress = String(address || "").trim();
  const rawChain = String(chain || "").trim().toUpperCase();
  if (!rawAddress) {
    return "";
  }
  switch (rawChain) {
    case "THOR":
      return `https://thorchain.net/address/${encodeURIComponent(rawAddress)}`;
    case "BTC":
      return `https://mempool.space/address/${encodeURIComponent(rawAddress)}`;
    case "LTC":
      return `https://litecoinspace.org/address/${encodeURIComponent(rawAddress)}`;
    case "BCH":
      return `https://blockchair.com/bitcoin-cash/address/${encodeURIComponent(rawAddress)}`;
    case "DOGE":
      return `https://blockchair.com/dogecoin/address/${encodeURIComponent(rawAddress)}`;
    case "ETH":
      return `https://etherscan.io/address/${encodeURIComponent(rawAddress)}`;
    case "BSC":
      return `https://bscscan.com/address/${encodeURIComponent(rawAddress)}`;
    case "BASE":
      return `https://basescan.org/address/${encodeURIComponent(rawAddress)}`;
    case "AVAX":
      return `https://snowtrace.io/address/${encodeURIComponent(rawAddress)}`;
    case "GAIA":
      return `https://www.mintscan.io/cosmos/address/${encodeURIComponent(rawAddress)}`;
    case "SOL":
      return `https://explorer.solana.com/address/${encodeURIComponent(rawAddress)}`;
    case "TRON":
      return `https://tronscan.org/#/address/${encodeURIComponent(rawAddress)}`;
    case "XRP":
      return `https://xrpscan.com/account/${encodeURIComponent(rawAddress)}`;
    default:
      return "";
  }
}
