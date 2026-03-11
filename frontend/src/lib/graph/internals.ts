import type {
  ActorGraphResponse,
  AddressAnnotation,
  AddressExplorerResponse,
  FlowAssetValue,
  FlowEdge,
  FlowEdgeTransaction,
  FlowNode,
  SupportingAction,
} from "../types";
import { GRAPH_FILTER_TXN_TYPES, type GraphFilterState, type GraphMetadata, type GraphTxnBucket } from "./types";

export type ExplorerMergeSource = Pick<
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

export function nodeAddress(node: Pick<FlowNode, "metrics"> | null | undefined) {
  const value = node?.metrics?.address;
  return typeof value === "string" ? value.trim() : "";
}

export function actionKey(action: SupportingAction) {
  return `${action.tx_id}|${action.action_key || action.action_class}|${action.from_node}|${action.to_node}`;
}

export function nodeMergeKey(node: FlowNode) {
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

export function edgeMergeKey(edge: FlowEdge, nodeAlias: Map<string, string>) {
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

export function cloneMergedEdge(edge: FlowEdge, canonicalID: string, nodeAlias: Map<string, string>) {
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

export function graphFilterMetadataFromResponse(
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

export function graphTxnTypeAllowed(
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

export function graphTxnBucket(actionClass: string, actionKey: string, actionLabel: string): GraphTxnBucket | "" {
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

export function graphTxnBucketLabel(bucket: string) {
  return GRAPH_FILTER_TXN_TYPES.find((item) => item.key === bucket)?.label || "";
}

export function graphVisibleEdgeKey(rawEdge: FlowEdge, from: string, to: string) {
  const actionClass = String(rawEdge.action_class || "").trim().toLowerCase();
  return actionClass === "ownership" ? `${from}|${to}|ownership` : `${from}|${to}|flow`;
}

export function graphItemChainSet(sourceNode: FlowNode | undefined, targetNode: FlowNode | undefined) {
  return uniqueStrings([rawNodeChain(sourceNode), rawNodeChain(targetNode)].filter(Boolean));
}

export function graphChainsAllowed(chainSet: string[], filterState: GraphFilterState) {
  const selected = new Set(filterState.selectedChains.map((chain) => String(chain || "").trim().toUpperCase()));
  if (!selected.size) {
    return !filterState.availableChains.length;
  }
  if (!chainSet.length) {
    return true;
  }
  return chainSet.some((chain) => selected.has(chain));
}

export function filterTransactionsByTime(transactions: FlowEdgeTransaction[], filterState: GraphFilterState) {
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

export function normalizeEdgeTransactions(edge: FlowEdge | undefined) {
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

export function mergeEdgeTransactions(existingTransactions: FlowEdgeTransaction[], incomingTransactions: FlowEdgeTransaction[]) {
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

export function summarizeTransactions(transactions: FlowEdgeTransaction[]) {
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

export function hiddenAddressSet(metadata: GraphMetadata) {
  const blocklistedAddresses = metadata.blocklist.map((entry) =>
    String(entry.normalized_address || "").toLowerCase()
  );
  const asgardAddresses = metadata.annotations
    .filter((annotation) => annotation.kind === "asgard_vault")
    .map((annotation) => String(annotation.normalized_address || "").toLowerCase());
  return new Set([...blocklistedAddresses, ...asgardAddresses]);
}

export function labelAnnotationMap(annotations: AddressAnnotation[]) {
  return new Map(
    annotations
      .filter((annotation) => annotation.kind === "label" && annotation.value !== null && annotation.value !== undefined)
      .map((annotation) => [
        String(annotation.normalized_address || "").toLowerCase(),
        String(annotation.value || ""),
      ])
  );
}

export function buildFrontierSeed(address: string, chain: string | undefined) {
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

export function uniqueSeeds(seeds: Array<{ address: string; chain: string; encoded: string }>) {
  const seen = new Map<string, { address: string; chain: string; encoded: string }>();
  seeds.forEach((seed) => {
    if (!seen.has(seed.encoded)) {
      seen.set(seed.encoded, seed);
    }
  });
  return Array.from(seen.values());
}

export function mergeLiveHoldingsStatus(current: string, incoming: string) {
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

export function normalizeISODateTime(value: string | Date | number | null | undefined) {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  const date = new Date(raw);
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}

export function normalizeGraphFilterNumber(value: unknown) {
  const raw = String(value ?? "").trim();
  if (!raw) {
    return null;
  }
  const number = Number(raw);
  return Number.isFinite(number) ? number : null;
}

export function clampISOToRange(value: string, minValue: string, maxValue: string) {
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

export function clampGraphFilterNumber(
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

export function chainSelectionsMatchAll(selectedChains: string[], availableChains: string[]) {
  const selected = uniqueStrings(selectedChains.map((item) => String(item || "").trim().toUpperCase())).sort();
  const available = uniqueStrings(availableChains.map((item) => String(item || "").trim().toUpperCase())).sort();
  if (selected.length !== available.length) {
    return false;
  }
  return selected.every((item, index) => item === available[index]);
}

export function timeSelectionsMatchFullRange(filters: GraphFilterState, graphMinTime: string, graphMaxTime: string) {
  const start = normalizeISODateTime(filters.startTime);
  const end = normalizeISODateTime(filters.endTime);
  return start === normalizeISODateTime(graphMinTime) && end === normalizeISODateTime(graphMaxTime);
}

export function valueSelectionsMatchFullRange(
  filters: GraphFilterState,
  graphMinTxnUSD: number | null,
  graphMaxTxnUSD: number | null
) {
  return (
    graphFilterNumbersEqual(filters.minTxnUSD, graphMinTxnUSD) &&
    graphFilterNumbersEqual(filters.maxTxnUSD, graphMaxTxnUSD)
  );
}

export function stringMetric(metrics: Record<string, unknown> | null | undefined, key: string) {
  const value = metrics?.[key];
  return typeof value === "string" ? value.trim() : "";
}

export function numberMetric(metrics: Record<string, unknown> | null | undefined, key: string) {
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

export function booleanMetric(metrics: Record<string, unknown> | null | undefined, key: string) {
  return Boolean(metrics?.[key]);
}

export function uniqueStrings(values: string[]) {
  return Array.from(new Set(values.filter(Boolean)));
}

export function uniqueNumbers(values: number[]) {
  return Array.from(new Set(values.filter((value) => Number.isFinite(value))));
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

function rawNodeChain(node: FlowNode | undefined) {
  return String(node?.chain || "").trim().toUpperCase();
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

function graphFilterNumbersEqual(left: number | null, right: number | null) {
  const a = normalizeGraphFilterNumber(left);
  const b = normalizeGraphFilterNumber(right);
  if (a === null || b === null) {
    return a === b;
  }
  return Math.abs(a - b) < 1e-9;
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
