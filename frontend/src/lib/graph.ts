import type {
  Actor,
  ActorGraphResponse,
  AddressAnnotation,
  AddressExplorerResponse,
  BlocklistedAddress,
  FlowAssetValue,
  FlowEdge,
  FlowEdgeTransaction,
  FlowNode,
  SupportingAction,
} from "./types";

export const CHAIN_LOGO_URLS: Record<string, string> = {
  THOR: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/thorchain/info/logo.png",
  BTC: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/bitcoin/info/logo.png",
  ETH: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/ethereum/info/logo.png",
  BSC: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/smartchain/info/logo.png",
  BASE: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/base/info/logo.png",
  AVAX: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/avalanchec/info/logo.png",
  LTC: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/litecoin/info/logo.png",
  BCH: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/bitcoincash/info/logo.png",
  DOGE: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/doge/info/logo.png",
  GAIA: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/cosmos/info/logo.png",
  SOL: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/solana/info/logo.png",
  TRON: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/tron/info/logo.png",
  XRP: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/ripple/info/logo.png",
};

export const GRAPH_FILTER_TXN_TYPES = [
  { key: "bond_unbond", label: "Bond/Unbond" },
  { key: "rebond", label: "Rebond" },
  { key: "transfer", label: "Send/Transfer" },
  { key: "swap", label: "Swap" },
] as const;

export type GraphTxnBucket = (typeof GRAPH_FILTER_TXN_TYPES)[number]["key"];

export interface GraphFilterState {
  initialized: boolean;
  isOpen: boolean;
  txnTypes: Record<GraphTxnBucket, boolean>;
  availableChains: string[];
  selectedChains: string[];
  graphMinTime: string;
  graphMaxTime: string;
  graphMinTxnUSD: number | null;
  graphMaxTxnUSD: number | null;
  startTime: string;
  endTime: string;
  minTxnUSD: number | null;
  maxTxnUSD: number | null;
}

export interface GraphMetadata {
  annotations: AddressAnnotation[];
  blocklist: BlocklistedAddress[];
}

export interface ActorGraphViewState {
  expandedActorIDs: number[];
  expandedExternalChains: string[];
}

export interface VisibleGraphNode extends FlowNode {
  raw_node_ids: string[];
  displayLabel: string;
  live_holdings_label: string;
  live_holdings_status: string;
  color: string;
  borderColor: string;
  chainLogo: string;
  inspect: Record<string, unknown>;
  pie1Color?: string;
  pie1Size?: number;
  pie2Color?: string;
  pie2Size?: number;
  pie3Color?: string;
  pie3Size?: number;
  pie4Color?: string;
  pie4Size?: number;
}

export interface VisibleGraphEdge extends FlowEdge {
  source: string;
  target: string;
  width: number;
  lineColor: string;
  edgeLabel: string;
  action_classes: string[];
  action_keys: string[];
  action_labels: string[];
  action_domains: string[];
  action_buckets: string[];
  validator_addresses: string[];
  validator_labels: string[];
  contract_types: string[];
  contract_protocols: string[];
  chain_set: string[];
  raw_edge_ids: string[];
  inspect: Record<string, unknown>;
}

export interface VisibleGraph {
  nodes: VisibleGraphNode[];
  edges: VisibleGraphEdge[];
}

export type GraphSelection =
  | { kind: "node"; node: VisibleGraphNode }
  | { kind: "edge"; edge: VisibleGraphEdge }
  | null;

interface VisibleNodeAccumulator {
  id: string;
  label: string;
  kind: string;
  chain: string;
  stage: string;
  depth: number;
  actor_ids: number[];
  shared: boolean;
  collapsed: boolean;
  raw_node_ids: string[];
  metrics: Record<string, unknown>;
  color?: string;
}

interface AssetBucket {
  asset: string;
  direction: string;
  amountRaw: string;
  usdSpot: number;
  tokenSymbol: string;
}

interface VisibleEdgeAccumulator {
  id: string;
  source: string;
  target: string;
  actionClass: string;
  actionKey: string;
  actionLabel: string;
  actionDomain: string;
  validatorAddress: string;
  validatorLabel: string;
  contractType: string;
  contractProtocol: string;
  usdSpot: number;
  actor_ids: number[];
  rawEdgeIDs: string[];
  txCount: number;
  txIDs: string[];
  assetTotals: Record<string, AssetBucket>;
  transactions: FlowEdgeTransaction[];
  chainSet: string[];
  actionClasses: string[];
  actionKeys: string[];
  actionLabels: string[];
  actionDomains: string[];
  txnBuckets: string[];
  validatorAddresses: string[];
  validatorLabels: string[];
  contractTypes: string[];
  contractProtocols: string[];
  width: number;
  inspect: Record<string, unknown>;
}

type ExplorerMergeSource = Pick<
  ActorGraphResponse,
  "nodes" | "edges" | "supporting_actions" | "warnings" | "stats"
> &
  Partial<
    Pick<
      AddressExplorerResponse,
      | "mode"
      | "raw_address"
      | "address"
      | "query"
      | "active_chains"
      | "seed_summaries"
      | "direction_required"
      | "run_label"
      | "loaded_actions"
      | "has_more"
      | "next_offset"
      | "total_estimate"
    >
  >;

type CytoscapeStyleBlock = {
  selector: string;
  style: Record<string, string | number>;
};

export function nodeAddress(node: Pick<FlowNode, "metrics"> | null | undefined) {
  const value = node?.metrics?.address;
  return typeof value === "string" ? value.trim() : "";
}

export function createGraphFilterState(): GraphFilterState {
  return {
    initialized: false,
    isOpen: false,
    txnTypes: {
      bond_unbond: true,
      rebond: true,
      transfer: true,
      swap: true,
    },
    availableChains: [],
    selectedChains: [],
    graphMinTime: "",
    graphMaxTime: "",
    graphMinTxnUSD: null,
    graphMaxTxnUSD: null,
    startTime: "",
    endTime: "",
    minTxnUSD: null,
    maxTxnUSD: null,
  };
}

export function cloneGraphFilterState(filterState: GraphFilterState) {
  return {
    ...filterState,
    txnTypes: { ...filterState.txnTypes },
    availableChains: [...filterState.availableChains],
    selectedChains: [...filterState.selectedChains],
  };
}

export function syncGraphFilterStateWithResponse(
  filterState: GraphFilterState,
  response: Pick<ActorGraphResponse | AddressExplorerResponse, "nodes" | "edges"> | null,
  options: { reset?: boolean } = {}
) {
  if (!response) {
    return;
  }

  const metadata = graphFilterMetadataFromResponse(response);
  const reset = Boolean(options.reset);
  const previousChains = [...filterState.availableChains];
  const previousMinTime = normalizeISODateTime(filterState.graphMinTime);
  const previousMaxTime = normalizeISODateTime(filterState.graphMaxTime);
  const previousMinTxnUSD = normalizeGraphFilterNumber(filterState.graphMinTxnUSD);
  const previousMaxTxnUSD = normalizeGraphFilterNumber(filterState.graphMaxTxnUSD);
  const selectedAllChains = chainSelectionsMatchAll(filterState.selectedChains, previousChains);
  const selectedFullRange = timeSelectionsMatchFullRange(filterState, previousMinTime, previousMaxTime);
  const selectedFullValueRange = valueSelectionsMatchFullRange(
    filterState,
    previousMinTxnUSD,
    previousMaxTxnUSD
  );

  filterState.availableChains = metadata.availableChains;
  filterState.graphMinTime = metadata.graphMinTime;
  filterState.graphMaxTime = metadata.graphMaxTime;
  filterState.graphMinTxnUSD = metadata.graphMinTxnUSD;
  filterState.graphMaxTxnUSD = metadata.graphMaxTxnUSD;

  if (reset || !filterState.initialized) {
    GRAPH_FILTER_TXN_TYPES.forEach((item) => {
      filterState.txnTypes[item.key] = true;
    });
    filterState.selectedChains = [...metadata.availableChains];
    filterState.startTime = metadata.graphMinTime;
    filterState.endTime = metadata.graphMaxTime;
    filterState.minTxnUSD = metadata.graphMinTxnUSD;
    filterState.maxTxnUSD = metadata.graphMaxTxnUSD;
    filterState.initialized = true;
    return;
  }

  if (selectedAllChains) {
    filterState.selectedChains = [...metadata.availableChains];
  } else {
    filterState.selectedChains = uniqueStrings(
      filterState.selectedChains.filter((chain) => metadata.availableChains.includes(chain))
    );
  }
  if (!filterState.selectedChains.length && metadata.availableChains.length) {
    filterState.selectedChains = [...metadata.availableChains];
  }

  if (selectedFullRange) {
    filterState.startTime = metadata.graphMinTime;
    filterState.endTime = metadata.graphMaxTime;
  } else {
    filterState.startTime = clampISOToRange(filterState.startTime, metadata.graphMinTime, metadata.graphMaxTime);
    filterState.endTime = clampISOToRange(filterState.endTime, metadata.graphMinTime, metadata.graphMaxTime);
    if (filterState.startTime && filterState.endTime && filterState.startTime > filterState.endTime) {
      filterState.startTime = metadata.graphMinTime;
      filterState.endTime = metadata.graphMaxTime;
    }
  }

  if (selectedFullValueRange) {
    filterState.minTxnUSD = metadata.graphMinTxnUSD;
    filterState.maxTxnUSD = metadata.graphMaxTxnUSD;
  } else {
    filterState.minTxnUSD = clampGraphFilterNumber(
      filterState.minTxnUSD,
      metadata.graphMinTxnUSD,
      metadata.graphMaxTxnUSD
    );
    filterState.maxTxnUSD = clampGraphFilterNumber(
      filterState.maxTxnUSD,
      metadata.graphMinTxnUSD,
      metadata.graphMaxTxnUSD
    );
    if (
      filterState.minTxnUSD !== null &&
      filterState.maxTxnUSD !== null &&
      filterState.minTxnUSD > filterState.maxTxnUSD
    ) {
      filterState.minTxnUSD = metadata.graphMinTxnUSD;
      filterState.maxTxnUSD = metadata.graphMaxTxnUSD;
    }
  }
}

export function graphFiltersAreActive(filterState: GraphFilterState | null | undefined) {
  if (!filterState) {
    return false;
  }
  const allTxnEnabled = GRAPH_FILTER_TXN_TYPES.every((item) => filterState.txnTypes[item.key] !== false);
  const allChainsSelected = chainSelectionsMatchAll(filterState.selectedChains, filterState.availableChains);
  const fullRangeSelected = timeSelectionsMatchFullRange(
    filterState,
    filterState.graphMinTime,
    filterState.graphMaxTime
  );
  const fullValueRangeSelected = valueSelectionsMatchFullRange(
    filterState,
    filterState.graphMinTxnUSD,
    filterState.graphMaxTxnUSD
  );
  return !(allTxnEnabled && allChainsSelected && fullRangeSelected && fullValueRangeSelected);
}

export function setGraphFilterDateValue(
  filterState: GraphFilterState,
  field: "startTime" | "endTime",
  localValue: string
) {
  const normalized = normalizeISODateTime(localValue ? new Date(localValue) : "");
  if (!normalized) {
    filterState[field] = field === "startTime" ? filterState.graphMinTime : filterState.graphMaxTime;
  } else {
    filterState[field] = clampISOToRange(normalized, filterState.graphMinTime, filterState.graphMaxTime);
  }
  if (filterState.startTime && filterState.endTime && filterState.startTime > filterState.endTime) {
    if (field === "startTime") {
      filterState.endTime = filterState.startTime;
    } else {
      filterState.startTime = filterState.endTime;
    }
  }
}

export function setGraphFilterNumberValue(
  filterState: GraphFilterState,
  field: "minTxnUSD" | "maxTxnUSD",
  rawValue: string
) {
  const normalized = normalizeGraphFilterNumber(rawValue);
  if (normalized === null) {
    filterState[field] = field === "minTxnUSD" ? filterState.graphMinTxnUSD : filterState.graphMaxTxnUSD;
  } else {
    filterState[field] = clampGraphFilterNumber(
      normalized,
      filterState.graphMinTxnUSD,
      filterState.graphMaxTxnUSD
    );
  }

  if (
    filterState.minTxnUSD !== null &&
    filterState.maxTxnUSD !== null &&
    filterState.minTxnUSD > filterState.maxTxnUSD
  ) {
    if (field === "minTxnUSD") {
      filterState.maxTxnUSD = filterState.minTxnUSD;
    } else {
      filterState.minTxnUSD = filterState.maxTxnUSD;
    }
  }
}

export function resetGraphFilters(filterState: GraphFilterState) {
  GRAPH_FILTER_TXN_TYPES.forEach((item) => {
    filterState.txnTypes[item.key] = true;
  });
  filterState.selectedChains = [...filterState.availableChains];
  filterState.startTime = filterState.graphMinTime;
  filterState.endTime = filterState.graphMaxTime;
  filterState.minTxnUSD = filterState.graphMinTxnUSD;
  filterState.maxTxnUSD = filterState.graphMaxTxnUSD;
}

export function formatGraphFilterNumber(value: number | null | undefined) {
  const normalized = normalizeGraphFilterNumber(value);
  if (normalized === null) {
    return "";
  }
  return Number.isInteger(normalized) ? String(normalized) : String(normalized);
}

export function graphStylesheet(mode: "actor" | "explorer"): CytoscapeStyleBlock[] {
  const base: CytoscapeStyleBlock[] = [
    {
      selector: "node",
      style: {
        label: "",
        color: "#f5fbff",
        "background-color": "data(color)",
        "background-image": "data(chainLogo)",
        "background-fit": "contain",
        "background-width": "60%",
        "background-height": "60%",
        "background-opacity": 0.3,
        "background-clip": "node",
        width: 58,
        height: 58,
        "font-size": 11,
        "font-weight": 700,
        "text-wrap": "wrap",
        "text-max-width": 110,
        "text-valign": "center",
        "text-halign": "center",
        "border-width": 2,
        "border-color": "data(borderColor)",
        "overlay-padding": 8,
        "pie-size": "76%",
        "pie-1-background-color": "data(pie1Color)",
        "pie-1-background-size": "data(pie1Size)",
        "pie-2-background-color": "data(pie2Color)",
        "pie-2-background-size": "data(pie2Size)",
        "pie-3-background-color": "data(pie3Color)",
        "pie-3-background-size": "data(pie3Size)",
        "pie-4-background-color": "data(pie4Color)",
        "pie-4-background-size": "data(pie4Size)",
      },
    },
    {
      selector: "node:selected",
      style: {
        "border-width": 4,
        "border-color": "#ffdd44",
        "overlay-color": "rgba(255,221,68,0.15)",
        "overlay-opacity": 0.3,
      },
    },
    {
      selector: "node[kind = 'actor']",
      style: {
        shape: "round-rectangle",
        width: 136,
        height: 56,
        "font-size": 13,
        "background-color": "#0d1b2a",
        "background-width": "50%",
        "background-height": "80%",
        "background-image-opacity": 1,
        "background-opacity": 1,
        "border-width": 4,
        "pie-size": "0%",
      },
    },
    {
      selector: "node[kind = 'pool']",
      style: {
        shape: "diamond",
        width: 84,
        height: 84,
        "background-color": "#1e4f8f",
      },
    },
    {
      selector: "node[kind = 'actor_address']",
      style: {
        shape: "ellipse",
        width: 62,
        height: 62,
        "background-color": "#0d1b2a",
        "background-opacity": 1,
        "background-width": "60%",
        "background-height": "60%",
        "background-image-opacity": 1,
        "border-width": 4,
        "pie-size": "0%",
      },
    },
    {
      selector: "node[kind = 'external_address']",
      style: {
        shape: "ellipse",
        width: 62,
        height: 62,
      },
    },
    {
      selector: "node[kind = 'node']",
      style: {
        shape: "octagon",
        width: 94,
        height: 94,
        "background-color": "#c86b1f",
        "border-color": "#ffe0b8",
        "border-width": 4,
        color: "#fff7ea",
        "font-size": 12,
      },
    },
    {
      selector: "node[kind = 'contract_address']",
      style: {
        shape: "round-rectangle",
        width: 108,
        height: 60,
        "background-color": "#915a2b",
      },
    },
    {
      selector: "node[kind = 'bond_address']",
      style: {
        shape: "tag",
        "background-color": "#654590",
      },
    },
    {
      selector: "node[kind = 'inbound'], node[kind = 'router']",
      style: {
        shape: "round-rectangle",
        "background-color": "#176666",
      },
    },
    {
      selector: "node[kind = 'external_cluster']",
      style: {
        shape: "barrel",
        width: 120,
        height: 56,
        "background-color": "#164a47",
      },
    },
    {
      selector: "edge",
      style: {
        label: "data(edgeLabel)",
        width: "data(width)",
        "line-color": "data(lineColor)",
        "target-arrow-color": "data(lineColor)",
        "target-arrow-shape": "triangle",
        "curve-style": "bezier",
        "arrow-scale": 0.95,
        opacity: 0.82,
        color: "#d9ecff",
        "font-size": 9,
        "text-wrap": "wrap",
        "text-max-width": 190,
        "text-rotation": "autorotate",
        "text-background-color": "rgba(7, 16, 31, 0.88)",
        "text-background-opacity": 1,
        "text-background-shape": "roundrectangle",
        "text-background-padding": "3px",
        "text-events": "no",
        "text-margin-y": -8,
      },
    },
    {
      selector: "edge[actionClass = 'ownership']",
      style: {
        label: "",
        width: 1.4,
        "line-style": "dashed",
        "line-color": "#7a94bb",
        "target-arrow-color": "#7a94bb",
        opacity: 0.5,
      },
    },
  ];

  if (mode === "explorer") {
    base.splice(2, 0, {
      selector: "node[kind = 'explorer_target']",
      style: {
        shape: "hexagon",
        width: 110,
        height: 100,
        "background-color": "#e67e22",
        "border-color": "#f5c76e",
        "border-width": 4,
        "font-size": 13,
      },
    });
  }

  return base;
}

export function graphLayoutNodeSize(mode: "actor" | "explorer", node: VisibleGraphNode) {
  if (mode === "explorer" && node.kind === "explorer_target") {
    return { width: 120, height: 108 };
  }
  if (node.kind === "actor") {
    return { width: 150, height: 64 };
  }
  if (node.kind === "pool") {
    return { width: 96, height: 96 };
  }
  return { width: 84, height: 72 };
}

export function graphLineColor(actionClass: string) {
  switch (actionClass) {
    case "liquidity":
      return "#78d6c4";
    case "swaps":
      return "#52a8ff";
    case "bonds":
      return "#d9a6ff";
    case "ownership":
      return "#7a94bb";
    default:
      return "#cfdcff";
  }
}

export function edgeWidth(usdSpot: number) {
  if (!usdSpot || usdSpot <= 0) {
    return 2.2;
  }
  return Math.min(12, 2 + Math.log10(usdSpot + 1) * 1.6);
}

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
      existing.actor_ids = uniqueNumbers(existing.actor_ids.concat(rawNode.actor_ids));
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

  return finalizeVisibleGraph(visibleNodes, visibleEdges, new Map(), labelAnnotations);
}

export function filterSupportingActions(
  actions: SupportingAction[],
  response: Pick<ActorGraphResponse | AddressExplorerResponse, "nodes">,
  filterState: GraphFilterState
) {
  const rawNodeByID = new Map(response.nodes.map((node) => [node.id, node]));
  const startTime = normalizeISODateTime(filterState.startTime);
  const endTime = normalizeISODateTime(filterState.endTime);
  const minTxnUSD = normalizeGraphFilterNumber(filterState.minTxnUSD);
  const maxTxnUSD = normalizeGraphFilterNumber(filterState.maxTxnUSD);

  return actions.filter((action) => {
    if (!graphTxnTypeAllowed(action.action_class, action.action_key, action.action_label, filterState)) {
      return false;
    }
    const chainSet = graphItemChainSet(
      rawNodeByID.get(String(action.from_node || "")),
      rawNodeByID.get(String(action.to_node || ""))
    );
    if (!graphChainsAllowed(chainSet, filterState)) {
      return false;
    }
    const when = normalizeISODateTime(action.time);
    if ((startTime || endTime) && !when) {
      return false;
    }
    if (startTime && when < startTime) {
      return false;
    }
    if (endTime && when > endTime) {
      return false;
    }
    const usdSpot = Number(action.usd_spot);
    if ((minTxnUSD !== null || maxTxnUSD !== null) && !Number.isFinite(usdSpot)) {
      return false;
    }
    if (minTxnUSD !== null && usdSpot < minTxnUSD) {
      return false;
    }
    if (maxTxnUSD !== null && usdSpot > maxTxnUSD) {
      return false;
    }
    return true;
  });
}

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
    existing.transactions = mergeEdgeTransactions(existing.transactions, normalizeEdgeTransactions(edge));
    const summary = summarizeTransactions(existing.transactions);
    existing.tx_ids = summary.tx_ids;
    existing.heights = summary.heights;
    existing.assets = summary.assets;
    existing.usd_spot = summary.usd_spot;
  }
  current.edges.forEach(addEdge);
  incoming.edges.forEach(addEdge);

  const actionMap = new Map<string, SupportingAction>();
  [...current.supporting_actions, ...incoming.supporting_actions].forEach((action) => {
    actionMap.set(actionKey(action), action);
  });

  const nodes = Array.from(nodeMap.values());
  const edges = Array.from(edgeMap.values());
  const supporting_actions = Array.from(actionMap.values());

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
    existing.confidence = Math.max(Number(existing.confidence || 0), Number(edge.confidence || 0));
    existing.transactions = mergeEdgeTransactions(existing.transactions, normalizeEdgeTransactions(edge));
    const summary = summarizeTransactions(existing.transactions);
    existing.tx_ids = summary.tx_ids;
    existing.heights = summary.heights;
    existing.assets = summary.assets;
    existing.usd_spot = summary.usd_spot;
  }
  current.edges.forEach(addEdge);
  incoming.edges.forEach(addEdge);

  const actionMap = new Map<string, SupportingAction>();
  [...current.supporting_actions, ...incoming.supporting_actions].forEach((action) => {
    actionMap.set(actionKey(action), action);
  });

  const nodes = Array.from(nodeMap.values());
  const edges = Array.from(edgeMap.values());
  const supporting_actions = Array.from(actionMap.values());

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

function actionKey(action: SupportingAction) {
  return `${action.tx_id}|${action.action_key || action.action_class}|${action.from_node}|${action.to_node}`;
}

function nodeMergeKey(node: FlowNode) {
  const address = nodeAddress(node).toLowerCase();
  const pool = stringMetric(node.metrics, "pool").toUpperCase();
  const chain = String(node.chain || "").trim().toUpperCase();
  const kind = String(node.kind || "").trim().toLowerCase();
  if (address) {
    return `${kind}|${chain}|${address}`;
  }
  if (pool) {
    return `${kind}|${pool}`;
  }
  if (kind === "actor" && node.actor_ids.length === 1) {
    return `actor|${node.actor_ids[0]}`;
  }
  return String(node.id || "");
}

function edgeMergeKey(edge: FlowEdge, nodeAlias: Map<string, string>) {
  const from = nodeAlias.get(edge.from) || edge.from;
  const to = nodeAlias.get(edge.to) || edge.to;
  let key = `${from}|${to}|${edge.action_key || edge.action_class}`;
  if (
    edge.validator_address &&
    String(edge.action_key || edge.action_class || "").toLowerCase().includes("rebond")
  ) {
    key += `|validator:${edge.validator_address}`;
  }
  return key;
}

function cloneMergedEdge(edge: FlowEdge, canonicalID: string, nodeAlias: Map<string, string>) {
  const transactions = mergeEdgeTransactions([], normalizeEdgeTransactions(edge));
  const summary = summarizeTransactions(transactions);
  return {
    ...edge,
    id: canonicalID,
    from: nodeAlias.get(edge.from) || edge.from,
    to: nodeAlias.get(edge.to) || edge.to,
    actor_ids: [...edge.actor_ids],
    transactions,
    tx_ids: summary.tx_ids,
    heights: summary.heights,
    assets: summary.assets,
    usd_spot: summary.usd_spot,
  };
}

function finalizeVisibleGraph(
  visibleNodes: Map<string, VisibleNodeAccumulator>,
  visibleEdges: Map<string, VisibleEdgeAccumulator>,
  actorByIDMap: Map<number, Actor>,
  labelAnnotations: Map<string, string>
): VisibleGraph {
  const nodes = Array.from(visibleNodes.values()).map((node) =>
    decorateVisibleNode(node, actorByIDMap, labelAnnotations)
  );

  const edges = Array.from(visibleEdges.values()).map((edge) => {
    const aggregatedAssets = Object.values(edge.assetTotals)
      .map((bucket) => ({
        asset: bucket.asset,
        direction: bucket.direction || "",
        amount_raw: bucket.amountRaw,
        usd_spot: bucket.usdSpot,
        token_symbol: bucket.tokenSymbol || "",
      }))
      .sort((left, right) => Number(right.usd_spot || 0) - Number(left.usd_spot || 0));
    const tokenSummary =
      edge.actionClass === "swaps"
        ? formatSwapTokenSummary(aggregatedAssets)
        : formatEdgeTokenSummary(aggregatedAssets);
    const usdSummary = formatUSD(edge.usdSpot);
    const edgeActionLabel = formatVisibleEdgeActionLabel(edge);
    const edgeLabel =
      edge.actionClass === "ownership" ? "" : `${edgeActionLabel} · ${tokenSummary}\n${usdSummary}`;

    return {
      id: edge.id,
      source: edge.source,
      target: edge.target,
      from: edge.source,
      to: edge.target,
      action_class: edge.actionClass,
      action_key: edge.actionKey,
      action_label: edgeActionLabel,
      action_domain: edge.actionDomain,
      validator_address: edge.validatorAddress || "",
      validator_label: edge.validatorLabel || "",
      contract_type: edge.contractType || "",
      contract_protocol: edge.contractProtocol || "",
      assets: aggregatedAssets,
      transactions: edge.transactions,
      usd_spot: edge.usdSpot,
      tx_ids: edge.txIDs,
      heights: [],
      actor_ids: uniqueNumbers(edge.actor_ids),
      confidence: 0,
      width: edge.width,
      lineColor: graphLineColor(edge.actionClass),
      edgeLabel,
      action_classes: edge.actionClasses,
      action_keys: edge.actionKeys,
      action_labels: edge.actionLabels,
      action_domains: edge.actionDomains,
      action_buckets: edge.txnBuckets,
      validator_addresses: edge.validatorAddresses,
      validator_labels: edge.validatorLabels,
      contract_types: edge.contractTypes,
      contract_protocols: edge.contractProtocols,
      chain_set: edge.chainSet,
      raw_edge_ids: edge.rawEdgeIDs,
      inspect: {
        from: edge.source,
        to: edge.target,
        action_class: edge.actionClass,
        action_key: edge.actionKey,
        action_label: edgeActionLabel,
        action_domain: edge.actionDomain,
        validator_address: edge.validatorAddress || "",
        validator_label: edge.validatorLabel || "",
        action_classes: edge.actionClasses,
        action_keys: edge.actionKeys,
        action_labels: edge.actionLabels,
        action_domains: edge.actionDomains,
        action_buckets: edge.txnBuckets,
        validator_addresses: edge.validatorAddresses,
        validator_labels: edge.validatorLabels,
        contract_type: edge.contractType,
        contract_protocol: edge.contractProtocol,
        usd_spot: edge.usdSpot,
        tx_count: edge.txCount,
        tx_ids: edge.txIDs,
        raw_edge_ids: edge.rawEdgeIDs,
        transactions: edge.transactions,
        chain_set: edge.chainSet,
        aggregated_assets: aggregatedAssets,
      },
    };
  });

  return { nodes, edges };
}

function decorateVisibleNode(
  node: VisibleNodeAccumulator,
  actorByIDMap: Map<number, Actor>,
  labelAnnotations: Map<string, string>
): VisibleGraphNode {
  const colors = node.actor_ids.map((id) => actorByIDMap.get(id)?.color).filter(Boolean) as string[];
  const baseColor = node.color || colors[0] || defaultNodeColor(node.kind);
  const pies = buildSharedPie(colors);
  const liveHoldingsAvailable = booleanMetric(node.metrics, "live_holdings_available");
  const liveHoldingsUSD = numberMetric(node.metrics, "live_holdings_usd_spot");
  const liveHoldingsStatus = stringMetric(node.metrics, "live_holdings_status").toLowerCase();
  const nodeTotalBondRaw = stringMetric(node.metrics, "node_total_bond");
  const liveHoldingsLabel =
    node.kind === "node" && nodeTotalBondRaw
      ? `${formatTokenAmountRaw(nodeTotalBondRaw)} RUNE`
      : liveHoldingsAvailable
      ? formatCompactUSD(liveHoldingsUSD)
      : liveHoldingsStatus === "error"
      ? "Unavailable"
      : "";
  const address = nodeAddress(node).toLowerCase();
  const customLabel = address ? labelAnnotations.get(address) : "";
  const displayLabel = customLabel || node.label || "";
  const showChainLogo = !["pool", "external_cluster"].includes(node.kind);
  const chainLogo = showChainLogo ? CHAIN_LOGO_URLS[node.chain] || "none" : "none";

  return {
    id: node.id,
    label: displayLabel,
    displayLabel,
    kind: node.kind,
    chain: node.chain,
    stage: node.stage,
    depth: node.depth,
    actor_ids: node.actor_ids,
    shared: node.shared,
    collapsed: node.collapsed,
    metrics: node.metrics,
    raw_node_ids: node.raw_node_ids,
    live_holdings_label: liveHoldingsLabel,
    live_holdings_status: liveHoldingsStatus,
    color: baseColor,
    chainLogo,
    borderColor:
      node.kind === "actor" || node.kind === "actor_address"
        ? baseColor
        : node.shared
        ? "#f4e7a3"
        : node.kind === "external_cluster"
        ? "#75d2ba"
        : "#a2c4ff",
    inspect: {
      id: node.id,
      label: node.label,
      kind: node.kind,
      chain: node.chain,
      stage: node.stage,
      depth: node.depth,
      actor_ids: node.actor_ids,
      raw_node_ids: node.raw_node_ids,
      metrics: node.metrics,
    },
    ...pies,
  };
}

function makeVisibleEdgeAdder(
  rawNodeByID: Map<string, FlowNode>,
  visibleEdges: Map<string, VisibleEdgeAccumulator>,
  ensureVisibleNode: (rawNode: FlowNode, mappedID: string | null) => void,
  mapNodeID: (rawNode: FlowNode | undefined) => string | null
) {
  return (rawEdge: FlowEdge & { chainSet?: string[] }) => {
    const sourceNode = rawNodeByID.get(rawEdge.from);
    const targetNode = rawNodeByID.get(rawEdge.to);
    const from = mapNodeID(sourceNode);
    const to = mapNodeID(targetNode);
    if (!sourceNode || !targetNode || !from || !to || from === to) {
      return false;
    }
    ensureVisibleNode(sourceNode, from);
    ensureVisibleNode(targetNode, to);

    const edgeID = graphVisibleEdgeKey(rawEdge, from, to);
    const existing = visibleEdges.get(edgeID) ?? {
      id: edgeID,
      source: from,
      target: to,
      actionClass: rawEdge.action_class,
      actionKey: rawEdge.action_key || rawEdge.action_class,
      actionLabel: rawEdge.action_label || rawEdge.action_class,
      actionDomain: rawEdge.action_domain || rawEdge.action_class,
      validatorAddress: rawEdge.validator_address || "",
      validatorLabel: rawEdge.validator_label || "",
      contractType: rawEdge.contract_type || "",
      contractProtocol: rawEdge.contract_protocol || "",
      usdSpot: 0,
      actor_ids: [],
      rawEdgeIDs: [],
      txCount: 0,
      txIDs: [],
      assetTotals: {},
      transactions: [],
      chainSet: [],
      actionClasses: [],
      actionKeys: [],
      actionLabels: [],
      actionDomains: [],
      txnBuckets: [],
      validatorAddresses: [],
      validatorLabels: [],
      contractTypes: [],
      contractProtocols: [],
      width: 0,
      inspect: {
        action_class: rawEdge.action_class,
        action_key: rawEdge.action_key || rawEdge.action_class,
        action_label: rawEdge.action_label || rawEdge.action_class,
        contract_type: rawEdge.contract_type || "",
        contract_protocol: rawEdge.contract_protocol || "",
        validator_address: rawEdge.validator_address || "",
        validator_label: rawEdge.validator_label || "",
        action_classes: [],
        action_keys: [],
        action_labels: [],
        action_domains: [],
        action_buckets: [],
        validator_addresses: [],
        validator_labels: [],
        chain_set: [],
        edges: [],
      },
    };

    existing.actor_ids = uniqueNumbers(existing.actor_ids.concat(rawEdge.actor_ids));
    existing.rawEdgeIDs = uniqueStrings(existing.rawEdgeIDs.concat(rawEdge.id));
    existing.actionClasses = uniqueStrings(
      existing.actionClasses.concat(String(rawEdge.action_class || "").trim()).filter(Boolean)
    );
    existing.actionKeys = uniqueStrings(
      existing.actionKeys.concat(String(rawEdge.action_key || rawEdge.action_class || "").trim()).filter(Boolean)
    );
    existing.actionLabels = uniqueStrings(
      existing.actionLabels.concat(String(rawEdge.action_label || rawEdge.action_class || "").trim()).filter(Boolean)
    );
    existing.actionDomains = uniqueStrings(
      existing.actionDomains.concat(String(rawEdge.action_domain || rawEdge.action_class || "").trim()).filter(Boolean)
    );
    const txnBucket = graphTxnBucket(rawEdge.action_class, rawEdge.action_key, rawEdge.action_label);
    if (txnBucket) {
      existing.txnBuckets = uniqueStrings(existing.txnBuckets.concat(txnBucket));
    }
    if (rawEdge.validator_address) {
      existing.validatorAddresses = uniqueStrings(
        existing.validatorAddresses.concat(String(rawEdge.validator_address).trim())
      );
    }
    if (rawEdge.validator_label) {
      existing.validatorLabels = uniqueStrings(
        existing.validatorLabels.concat(String(rawEdge.validator_label).trim())
      );
    }
    if (rawEdge.contract_type) {
      existing.contractTypes = uniqueStrings(
        existing.contractTypes.concat(String(rawEdge.contract_type).trim())
      );
    }
    if (rawEdge.contract_protocol) {
      existing.contractProtocols = uniqueStrings(
        existing.contractProtocols.concat(String(rawEdge.contract_protocol).trim())
      );
    }

    const resolved = resolveVisibleEdgeMetadata(existing);
    existing.actionClass = resolved.actionClass;
    existing.actionKey = resolved.actionKey;
    existing.actionLabel = resolved.actionLabel;
    existing.actionDomain = resolved.actionDomain;
    existing.validatorAddress = resolved.validatorAddress;
    existing.validatorLabel = resolved.validatorLabel;
    existing.contractType = resolved.contractType;
    existing.contractProtocol = resolved.contractProtocol;
    existing.transactions = mergeEdgeTransactions(existing.transactions, rawEdge.transactions || []);

    const summary = summarizeTransactions(existing.transactions);
    existing.usdSpot = summary.usd_spot;
    existing.txIDs = summary.tx_ids;
    existing.txCount = Math.max(existing.txIDs.length, existing.transactions.length);
    existing.chainSet = uniqueStrings(existing.chainSet.concat(rawEdge.chainSet || []));
    existing.inspect.action_class = existing.actionClass;
    existing.inspect.action_key = existing.actionKey;
    existing.inspect.action_label = existing.actionLabel;
    existing.inspect.action_domain = existing.actionDomain;
    existing.inspect.contract_type = existing.contractType;
    existing.inspect.contract_protocol = existing.contractProtocol;
    existing.inspect.validator_address = existing.validatorAddress || "";
    existing.inspect.validator_label = existing.validatorLabel || "";
    existing.inspect.action_classes = resolved.actionClasses;
    existing.inspect.action_keys = resolved.actionKeys;
    existing.inspect.action_labels = resolved.actionLabels;
    existing.inspect.action_domains = resolved.actionDomains;
    existing.inspect.action_buckets = resolved.txnBuckets;
    existing.inspect.validator_addresses = resolved.validatorAddresses;
    existing.inspect.validator_labels = resolved.validatorLabels;
    existing.inspect.chain_set = existing.chainSet;
    existing.assetTotals = {};
    summary.assets.forEach((assetValue) => {
      const asset = assetValue.asset || "THOR.RUNE";
      const direction = String(assetValue.direction || "").toLowerCase();
      existing.assetTotals[`${asset}|${direction}`] = {
        asset,
        direction,
        amountRaw: String(assetValue.amount_raw || "0"),
        usdSpot: Number(assetValue.usd_spot || 0),
        tokenSymbol: String(assetValue.token_symbol || ""),
      };
    });
    const existingEdges = existing.inspect.edges;
    if (Array.isArray(existingEdges)) {
      existingEdges.push(rawEdge);
    }
    existing.width = edgeWidth(existing.usdSpot);
    visibleEdges.set(edgeID, existing);
    return true;
  };
}

function resolveVisibleEdgeMetadata(edge: VisibleEdgeAccumulator) {
  const actionClasses = uniqueStrings(edge.actionClasses);
  const actionKeys = uniqueStrings(edge.actionKeys);
  const actionLabels = uniqueStrings(edge.actionLabels);
  const actionDomains = uniqueStrings(edge.actionDomains);
  const txnBuckets = uniqueStrings(edge.txnBuckets);
  const validatorAddresses = uniqueStrings(edge.validatorAddresses);
  const validatorLabels = uniqueStrings(edge.validatorLabels);
  const contractTypes = uniqueStrings(edge.contractTypes);
  const contractProtocols = uniqueStrings(edge.contractProtocols);

  let actionClass = actionClasses.length === 1 ? actionClasses[0] : actionClasses.length ? "mixed" : "";
  let actionKey = actionKeys.length === 1 ? actionKeys[0] : actionKeys.length ? "multiple" : "";
  let actionDomain = actionDomains.length === 1 ? actionDomains[0] : actionDomains.length ? "multiple" : "";
  let actionLabel = actionLabels.length === 1 ? actionLabels[0] : "";

  if (!actionLabel) {
    const bucketLabels = txnBuckets.map(graphTxnBucketLabel).filter(Boolean);
    actionLabel = summarizeGraphLabels(bucketLabels.length ? bucketLabels : actionLabels, 2);
  }
  if (!actionLabel && actionClasses.length === 1) {
    actionLabel = actionClasses[0];
  }
  if (!actionLabel) {
    actionLabel = "Transactions";
  }

  return {
    actionClass,
    actionKey,
    actionLabel,
    actionDomain,
    validatorAddress: validatorAddresses.length === 1 ? validatorAddresses[0] : "",
    validatorLabel: validatorLabels.length === 1 ? validatorLabels[0] : "",
    contractType: contractTypes.length === 1 ? contractTypes[0] : "",
    contractProtocol: contractProtocols.length === 1 ? contractProtocols[0] : "",
    actionClasses,
    actionKeys,
    actionLabels,
    actionDomains,
    txnBuckets,
    validatorAddresses,
    validatorLabels,
    contractTypes,
    contractProtocols,
  };
}

function graphFilterMetadataFromResponse(
  response: Pick<ActorGraphResponse | AddressExplorerResponse, "nodes" | "edges">
) {
  const availableChains = uniqueStrings(
    response.nodes.map((node) => String(node.chain || "").trim().toUpperCase()).filter(Boolean)
  ).sort();
  let graphMinTime = "";
  let graphMaxTime = "";
  let graphMinTxnUSD: number | null = null;
  let graphMaxTxnUSD: number | null = null;

  response.edges.forEach((edge) => {
    normalizeEdgeTransactions(edge).forEach((tx) => {
      const when = normalizeISODateTime(tx.time);
      if (when) {
        if (!graphMinTime || when < graphMinTime) {
          graphMinTime = when;
        }
        if (!graphMaxTime || when > graphMaxTime) {
          graphMaxTime = when;
        }
      }
      const usdSpot = Number(tx.usd_spot);
      if (!Number.isFinite(usdSpot)) {
        return;
      }
      if (graphMinTxnUSD === null || usdSpot < graphMinTxnUSD) {
        graphMinTxnUSD = usdSpot;
      }
      if (graphMaxTxnUSD === null || usdSpot > graphMaxTxnUSD) {
        graphMaxTxnUSD = usdSpot;
      }
    });
  });

  return { availableChains, graphMinTime, graphMaxTime, graphMinTxnUSD, graphMaxTxnUSD };
}

function graphTxnTypeAllowed(
  actionClass: string,
  actionKey: string,
  actionLabel: string,
  filterState: GraphFilterState
) {
  const bucket = graphTxnBucket(actionClass, actionKey, actionLabel);
  if (!bucket) {
    return true;
  }
  return filterState.txnTypes[bucket] !== false;
}

function graphTxnBucket(actionClass: string, actionKey: string, actionLabel: string): GraphTxnBucket | "" {
  const normalizedClass = String(actionClass || "").trim().toLowerCase();
  switch (normalizedClass) {
    case "bonds":
      return isRebondGraphAction(normalizedClass, actionKey, actionLabel) ? "rebond" : "bond_unbond";
    case "transfers":
      return "transfer";
    case "swaps":
      return "swap";
    default:
      return "";
  }
}

function graphTxnBucketLabel(bucket: string) {
  return GRAPH_FILTER_TXN_TYPES.find((item) => item.key === bucket)?.label || "";
}

function isRebondGraphAction(actionClass: string, actionKey: string, actionLabel: string) {
  if (String(actionClass || "").trim().toLowerCase() !== "bonds") {
    return false;
  }
  const key = `${String(actionKey || "").trim().toLowerCase()} ${String(actionLabel || "")
    .trim()
    .toLowerCase()}`;
  return key.includes("rebond");
}

function graphVisibleEdgeKey(rawEdge: FlowEdge, from: string, to: string) {
  const actionClass = String(rawEdge.action_class || "").trim().toLowerCase();
  return actionClass === "ownership" ? `${from}|${to}|ownership` : `${from}|${to}|flow`;
}

function graphItemChainSet(sourceNode: FlowNode | undefined, targetNode: FlowNode | undefined) {
  return uniqueStrings([rawNodeChain(sourceNode), rawNodeChain(targetNode)].filter(Boolean));
}

function rawNodeChain(node: FlowNode | undefined) {
  return String(node?.chain || "").trim().toUpperCase();
}

function graphChainsAllowed(chainSet: string[], filterState: GraphFilterState) {
  const selected = new Set(filterState.selectedChains.map((chain) => String(chain || "").trim().toUpperCase()));
  if (!selected.size) {
    return !filterState.availableChains.length;
  }
  if (!chainSet.length) {
    return true;
  }
  return chainSet.some((chain) => selected.has(chain));
}

function filterTransactionsByTime(transactions: FlowEdgeTransaction[], filterState: GraphFilterState) {
  const startTime = normalizeISODateTime(filterState.startTime);
  const endTime = normalizeISODateTime(filterState.endTime);
  const minTxnUSD = normalizeGraphFilterNumber(filterState.minTxnUSD);
  const maxTxnUSD = normalizeGraphFilterNumber(filterState.maxTxnUSD);

  return transactions.filter((tx) => {
    const when = normalizeISODateTime(tx.time);
    if ((startTime || endTime) && !when) {
      return false;
    }
    if (startTime && when < startTime) {
      return false;
    }
    if (endTime && when > endTime) {
      return false;
    }
    const usdSpot = Number(tx.usd_spot);
    if ((minTxnUSD !== null || maxTxnUSD !== null) && !Number.isFinite(usdSpot)) {
      return false;
    }
    if (minTxnUSD !== null && usdSpot < minTxnUSD) {
      return false;
    }
    if (maxTxnUSD !== null && usdSpot > maxTxnUSD) {
      return false;
    }
    return true;
  });
}

function cloneEdgeTransaction(tx: FlowEdgeTransaction, index = 0): FlowEdgeTransaction {
  return {
    tx_id: String(tx.tx_id || edgeTransactionKey(tx, index)),
    height: Number(tx.height || 0),
    time: normalizeISODateTime(tx.time),
    usd_spot: Number(tx.usd_spot || 0),
    assets: mergeFlowAssetValues([], tx.assets || []),
  };
}

function normalizeEdgeTransactions(edge: FlowEdge | undefined) {
  if (!edge) {
    return [];
  }
  if (!Array.isArray(edge.transactions) || !edge.transactions.length) {
    return [
      {
        tx_id: String(edge.tx_ids[0] || edge.id || ""),
        height: Number(edge.heights[0] || 0),
        time: "",
        usd_spot: Number(edge.usd_spot || 0),
        assets: mergeFlowAssetValues([], edge.assets || []),
      },
    ];
  }
  return edge.transactions.map((tx, index) => cloneEdgeTransaction(tx, index));
}

function mergeEdgeTransactions(existingTransactions: FlowEdgeTransaction[], incomingTransactions: FlowEdgeTransaction[]) {
  const txMap = new Map<string, FlowEdgeTransaction>();

  existingTransactions.forEach((tx, index) => {
    const cloned = cloneEdgeTransaction(tx, index);
    txMap.set(edgeTransactionKey(cloned, index), cloned);
  });
  incomingTransactions.forEach((tx, index) => {
    const cloned = cloneEdgeTransaction(tx, index);
    const key = edgeTransactionKey(cloned, index);
    const existing = txMap.get(key);
    if (!existing) {
      txMap.set(key, cloned);
      return;
    }
    if (!existing.height || (cloned.height && cloned.height < existing.height)) {
      existing.height = cloned.height || existing.height;
    }
    if (!existing.time || (cloned.time && cloned.time < existing.time)) {
      existing.time = cloned.time || existing.time;
    }
    existing.usd_spot = Number(existing.usd_spot || 0) + Number(cloned.usd_spot || 0);
    existing.assets = mergeFlowAssetValues(existing.assets, cloned.assets);
    txMap.set(key, existing);
  });

  return Array.from(txMap.values()).sort((left, right) => {
    if (left.time === right.time) {
      return String(left.tx_id || "").localeCompare(String(right.tx_id || ""));
    }
    if (!left.time) {
      return 1;
    }
    if (!right.time) {
      return -1;
    }
    return left.time.localeCompare(right.time);
  });
}

function summarizeTransactions(transactions: FlowEdgeTransaction[]) {
  const txIDs: string[] = [];
  const heights: number[] = [];
  let usdSpot = 0;
  let assets: FlowAssetValue[] = [];

  transactions.forEach((tx) => {
    usdSpot += Number(tx.usd_spot || 0);
    const txID = String(tx.tx_id || "").trim();
    if (txID) {
      txIDs.push(txID);
    }
    const height = Number(tx.height || 0);
    if (Number.isFinite(height) && height > 0) {
      heights.push(height);
    }
    assets = mergeFlowAssetValues(assets, tx.assets || []);
  });

  return {
    usd_spot: usdSpot,
    tx_ids: uniqueStrings(txIDs).sort(),
    heights: uniqueNumbers(heights).sort((left, right) => left - right),
    assets,
  };
}

function mergeFlowAssetValues(targetAssets: FlowAssetValue[], incomingAssets: FlowAssetValue[]) {
  const assetMap = new Map<string, FlowAssetValue>();

  targetAssets.forEach((asset) => {
    const cloned = cloneFlowAssetValue(asset);
    assetMap.set(`${cloned.asset}|${cloned.direction}`, cloned);
  });

  incomingAssets.forEach((asset) => {
    const cloned = cloneFlowAssetValue(asset);
    const key = `${cloned.asset}|${cloned.direction}`;
    const existing = assetMap.get(key);
    if (!existing) {
      assetMap.set(key, cloned);
      return;
    }
    existing.amount_raw = addRawAmountStrings(existing.amount_raw, cloned.amount_raw);
    existing.usd_spot = Number(existing.usd_spot || 0) + Number(cloned.usd_spot || 0);
    if (!existing.asset_kind) existing.asset_kind = cloned.asset_kind;
    if (!existing.token_standard) existing.token_standard = cloned.token_standard;
    if (!existing.token_address) existing.token_address = cloned.token_address;
    if (!existing.token_symbol) existing.token_symbol = cloned.token_symbol;
    if (!existing.token_name) existing.token_name = cloned.token_name;
    if (!existing.token_decimals) existing.token_decimals = cloned.token_decimals;
    assetMap.set(key, existing);
  });

  return Array.from(assetMap.values()).sort((left, right) => Number(right.usd_spot || 0) - Number(left.usd_spot || 0));
}

function cloneFlowAssetValue(asset: FlowAssetValue): FlowAssetValue {
  return {
    asset: String(asset.asset || ""),
    amount_raw: String(asset.amount_raw || "0"),
    usd_spot: Number(asset.usd_spot || 0),
    direction: String(asset.direction || "").toLowerCase(),
    asset_kind: String(asset.asset_kind || ""),
    token_standard: String(asset.token_standard || ""),
    token_address: String(asset.token_address || ""),
    token_symbol: String(asset.token_symbol || ""),
    token_name: String(asset.token_name || ""),
    token_decimals: Number(asset.token_decimals || 0),
  };
}

function edgeTransactionKey(tx: FlowEdgeTransaction, index = 0) {
  const txID = String(tx.tx_id || "").trim();
  if (txID) {
    return txID;
  }
  return `${String(tx.height || 0)}|${normalizeISODateTime(tx.time)}|${index}`;
}

function summarizeGraphLabels(labels: string[], maxVisible = 2) {
  const uniqueLabels = uniqueStrings(labels.map((label) => String(label || "").trim()).filter(Boolean));
  if (!uniqueLabels.length) {
    return "";
  }
  if (uniqueLabels.length <= maxVisible) {
    return uniqueLabels.join(" + ");
  }
  return `${uniqueLabels.slice(0, maxVisible).join(" + ")} +${uniqueLabels.length - maxVisible} more`;
}

function formatVisibleEdgeActionLabel(edge: VisibleEdgeAccumulator) {
  if (String(edge.actionClass || "").trim().toLowerCase() === "ownership") {
    return "";
  }

  let label = String(edge.actionLabel || "").trim() || "Transactions";
  const txCount = Math.max(Number(edge.txCount || 0), edge.transactions.length);
  const mixedKinds =
    uniqueStrings(edge.actionClasses).length > 1 ||
    uniqueStrings(edge.actionLabels).length > 1 ||
    uniqueStrings(edge.rawEdgeIDs).length > 1;
  if (txCount > 1 && mixedKinds) {
    label = `${label} (${txCount} txns)`;
  }

  const validatorAddresses = uniqueStrings(edge.validatorAddresses);
  const validatorLabels = uniqueStrings(edge.validatorLabels);
  const validatorCount = Math.max(validatorAddresses.length, validatorLabels.length);
  if (validatorCount === 1) {
    const validator = String(edge.validatorLabel || edge.validatorAddress || "").trim();
    if (validator && !label.toLowerCase().includes(" via ")) {
      label = `${label} via ${validator}`;
    }
  } else if (validatorCount > 1) {
    label = `${label} via ${validatorCount} validators`;
  }

  return label;
}

function formatEdgeTokenSummary(assets: FlowAssetValue[]) {
  if (!assets.length) {
    return "amount unavailable";
  }
  const primary = assets[0];
  const displayAsset = formatAssetDisplay(primary.asset, primary.token_symbol);
  const primaryText = `${formatTokenAmountRaw(primary.amount_raw)} ${displayAsset}`;
  if (assets.length === 1) {
    return primaryText;
  }
  return `${primaryText} +${assets.length - 1} more`;
}

function formatSwapTokenSummary(assets: FlowAssetValue[]) {
  if (!assets.length) {
    return "amount unavailable";
  }
  const inputs = assets.filter((asset) => String(asset.direction || "").toLowerCase() === "in");
  const outputs = assets.filter((asset) => String(asset.direction || "").toLowerCase() === "out");
  if (!inputs.length || !outputs.length) {
    return formatEdgeTokenSummary(assets);
  }

  const input = inputs[0];
  const output = outputs[0];
  const inputText = `${formatTokenAmountRaw(input.amount_raw)} ${formatAssetDisplay(input.asset, input.token_symbol)}`;
  const outputText = `${formatTokenAmountRaw(output.amount_raw)} ${formatAssetDisplay(output.asset, output.token_symbol)}`;
  const extras: string[] = [];
  if (inputs.length > 1) {
    extras.push(`+${inputs.length - 1} in`);
  }
  if (outputs.length > 1) {
    extras.push(`+${outputs.length - 1} out`);
  }
  if (!extras.length) {
    return `${inputText} -> ${outputText}`;
  }
  return `${inputText} -> ${outputText} (${extras.join(", ")})`;
}

function formatAssetDisplay(asset: string, tokenSymbol?: string) {
  const symbol = cleanAssetName(asset, tokenSymbol);
  const chain = asset.split(".")[0] || "";
  return chain ? `${chain}.${symbol}` : symbol;
}

function cleanAssetName(asset: string, tokenSymbol?: string) {
  if (tokenSymbol) {
    return tokenSymbol;
  }
  const dotIndex = asset.indexOf(".");
  if (dotIndex < 0) {
    return asset;
  }
  const afterDot = asset.slice(dotIndex + 1);
  const dashIndex = afterDot.indexOf("-");
  return dashIndex < 0 ? afterDot : afterDot.slice(0, dashIndex);
}

function buildSharedPie(colors: string[]) {
  const out: Partial<VisibleGraphNode> = {};
  const unique = uniqueStrings(colors).slice(0, 4);
  if (!unique.length) {
    return out;
  }
  const size = Math.floor(100 / unique.length);
  unique.forEach((color, index) => {
    const pieIndex = index + 1;
    out[`pie${pieIndex}Color` as keyof VisibleGraphNode] = color as never;
    out[`pie${pieIndex}Size` as keyof VisibleGraphNode] = size as never;
  });
  return out;
}

function defaultNodeColor(kind: string) {
  switch (kind) {
    case "pool":
      return "#2a5ea3";
    case "node":
      return "#c86b1f";
    case "contract_address":
      return "#915a2b";
    case "bond_address":
      return "#694b93";
    case "inbound":
    case "router":
      return "#176666";
    case "external_cluster":
      return "#164a47";
    default:
      return "#5f86be";
  }
}

function formatUSD(value: number) {
  if (!value) {
    return "0";
  }
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(value);
}

function formatCompactUSD(value: number) {
  const num = Number(value || 0);
  if (!Number.isFinite(num)) {
    return "$0";
  }
  const abs = Math.abs(num);
  if (abs >= 1_000_000_000) {
    return `$${(num / 1_000_000_000).toFixed(1)}B`;
  }
  if (abs >= 1_000_000) {
    return `$${(num / 1_000_000).toFixed(1)}M`;
  }
  if (abs >= 1_000) {
    return `$${(num / 1_000).toFixed(1)}K`;
  }
  return `$${Math.round(num)}`;
}

function formatTokenAmountRaw(amountRaw: string) {
  const value = toBigIntOrNull(amountRaw);
  if (value === null) {
    return String(amountRaw || "0");
  }
  const negative = value < 0n;
  const abs = negative ? -value : value;
  const whole = abs / 100000000n;
  const frac = abs % 100000000n;
  const fracText = frac.toString().padStart(8, "0").replace(/0+$/, "");
  let output = fracText ? `${whole}.${fracText}` : `${whole}`;
  if (negative) {
    output = `-${output}`;
  }
  return output;
}

function toBigIntOrNull(raw: unknown) {
  const value = String(raw ?? "").trim();
  if (!/^-?\d+$/.test(value)) {
    return null;
  }
  try {
    return BigInt(value);
  } catch {
    return null;
  }
}

function addRawAmountStrings(left: string, right: string) {
  const leftBig = toBigIntOrNull(left);
  const rightBig = toBigIntOrNull(right);
  if (leftBig !== null && rightBig !== null) {
    return (leftBig + rightBig).toString();
  }
  const leftNum = Number.parseFloat(String(left ?? "0")) || 0;
  const rightNum = Number.parseFloat(String(right ?? "0")) || 0;
  return String(leftNum + rightNum);
}

function hiddenAddressSet(metadata: GraphMetadata) {
  const blocklistedAddresses = metadata.blocklist.map((entry) =>
    String(entry.normalized_address || "").toLowerCase()
  );
  const asgardAddresses = metadata.annotations
    .filter((annotation) => annotation.kind === "asgard_vault")
    .map((annotation) => String(annotation.normalized_address || "").toLowerCase());
  return new Set([...blocklistedAddresses, ...asgardAddresses]);
}

function labelAnnotationMap(annotations: AddressAnnotation[]) {
  return new Map(
    annotations
      .filter((annotation) => annotation.kind === "label" && annotation.value !== null && annotation.value !== undefined)
      .map((annotation) => [
        String(annotation.normalized_address || "").toLowerCase(),
        String(annotation.value || ""),
      ])
  );
}

function buildFrontierSeed(address: string, chain: string | undefined) {
  const rawAddress = String(address || "").trim();
  if (!rawAddress) {
    return null;
  }
  const rawChain = String(chain || "").trim().toUpperCase();
  return {
    address: rawAddress,
    chain: rawChain,
    encoded: rawChain ? `${rawChain}|${rawAddress}` : rawAddress,
  };
}

function uniqueSeeds(seeds: Array<{ address: string; chain: string; encoded: string }>) {
  const seen = new Map<string, { address: string; chain: string; encoded: string }>();
  seeds.forEach((seed) => {
    if (!seen.has(seed.encoded)) {
      seen.set(seed.encoded, seed);
    }
  });
  return Array.from(seen.values());
}

function mergeLiveHoldingsStatus(current: string, incoming: string) {
  const currentStatus = String(current || "").trim().toLowerCase();
  const incomingStatus = String(incoming || "").trim().toLowerCase();
  if (currentStatus === "available" || incomingStatus === "available") {
    return "available";
  }
  if (currentStatus === "error" || incomingStatus === "error") {
    return "error";
  }
  return currentStatus || incomingStatus;
}

function normalizeISODateTime(value: string | Date | number | null | undefined) {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  const date = new Date(raw);
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}

function normalizeGraphFilterNumber(value: unknown) {
  const raw = String(value ?? "").trim();
  if (!raw) {
    return null;
  }
  const number = Number(raw);
  return Number.isFinite(number) ? number : null;
}

function clampISOToRange(value: string, minValue: string, maxValue: string) {
  const normalized = normalizeISODateTime(value);
  if (!normalized) {
    return "";
  }
  const minISO = normalizeISODateTime(minValue);
  const maxISO = normalizeISODateTime(maxValue);
  let output = normalized;
  if (minISO && output < minISO) {
    output = minISO;
  }
  if (maxISO && output > maxISO) {
    output = maxISO;
  }
  return output;
}

function clampGraphFilterNumber(
  value: number | null,
  minValue: number | null,
  maxValue: number | null
) {
  const normalized = normalizeGraphFilterNumber(value);
  if (normalized === null) {
    return null;
  }
  const minNumber = normalizeGraphFilterNumber(minValue);
  const maxNumber = normalizeGraphFilterNumber(maxValue);
  let output = normalized;
  if (minNumber !== null && output < minNumber) {
    output = minNumber;
  }
  if (maxNumber !== null && output > maxNumber) {
    output = maxNumber;
  }
  return output;
}

function chainSelectionsMatchAll(selectedChains: string[], availableChains: string[]) {
  const selected = uniqueStrings(selectedChains.map((item) => String(item || "").trim().toUpperCase())).sort();
  const available = uniqueStrings(availableChains.map((item) => String(item || "").trim().toUpperCase())).sort();
  if (selected.length !== available.length) {
    return false;
  }
  return selected.every((item, index) => item === available[index]);
}

function timeSelectionsMatchFullRange(filters: GraphFilterState, graphMinTime: string, graphMaxTime: string) {
  const start = normalizeISODateTime(filters.startTime);
  const end = normalizeISODateTime(filters.endTime);
  return start === normalizeISODateTime(graphMinTime) && end === normalizeISODateTime(graphMaxTime);
}

function graphFilterNumbersEqual(left: number | null, right: number | null) {
  const a = normalizeGraphFilterNumber(left);
  const b = normalizeGraphFilterNumber(right);
  if (a === null || b === null) {
    return a === b;
  }
  return Math.abs(a - b) < 1e-9;
}

function valueSelectionsMatchFullRange(
  filters: GraphFilterState,
  graphMinTxnUSD: number | null,
  graphMaxTxnUSD: number | null
) {
  return (
    graphFilterNumbersEqual(filters.minTxnUSD, graphMinTxnUSD) &&
    graphFilterNumbersEqual(filters.maxTxnUSD, graphMaxTxnUSD)
  );
}

function stringMetric(metrics: Record<string, unknown> | null | undefined, key: string) {
  const value = metrics?.[key];
  return typeof value === "string" ? value.trim() : "";
}

function numberMetric(metrics: Record<string, unknown> | null | undefined, key: string) {
  const value = metrics?.[key];
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function booleanMetric(metrics: Record<string, unknown> | null | undefined, key: string) {
  return Boolean(metrics?.[key]);
}

function uniqueStrings(values: string[]) {
  return Array.from(new Set(values.filter(Boolean)));
}

function uniqueNumbers(values: number[]) {
  return Array.from(new Set(values.filter((value) => Number.isFinite(value))));
}
