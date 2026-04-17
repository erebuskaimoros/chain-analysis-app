import type { Actor, FlowAssetValue, FlowEdge, FlowEdgeTransaction, FlowNode } from "../types";
import {
  booleanMetric,
  graphTxnBucket,
  graphTxnBucketLabel,
  graphVisibleEdgeKey,
  mergeEdgeTransactions,
  numberMetric,
  stringMetric,
  summarizeTransactions,
  uniqueNumbers,
  uniqueStrings,
} from "./internals";
import { edgeWidth, graphLineColor } from "./presentation";
import { CHAIN_LOGO_URLS, type VisibleGraph, type VisibleGraphNode } from "./types";

export interface VisibleNodeAccumulator {
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

export interface VisibleEdgeAccumulator {
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
  sourceProtocols: string[];
  width: number;
  inspect: Record<string, unknown>;
}

export function finalizeVisibleGraph(
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
      source_protocols: edge.sourceProtocols,
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
        source_protocols: edge.sourceProtocols,
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

export function makeVisibleEdgeAdder(
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
      sourceProtocols: [],
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
        source_protocols: [],
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
    existing.sourceProtocols = uniqueStrings(
      existing.sourceProtocols.concat((rawEdge.source_protocols || []).map((protocol) => String(protocol || "").trim().toUpperCase()).filter(Boolean))
    );

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
    existing.inspect.source_protocols = existing.sourceProtocols;
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

export function defaultNodeColor(kind: string) {
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
  const hasMeaningfulLiveHoldings = liveHoldingsAvailable && Math.round(liveHoldingsUSD) > 0;
  const liveHoldingsLabel =
    hasMeaningfulLiveHoldings
      ? formatCompactUSD(liveHoldingsUSD)
      : node.kind === "node" && nodeTotalBondRaw
      ? `${formatTokenAmountRaw(nodeTotalBondRaw)} RUNE`
      : "";
  const address = stringMetric(node.metrics, "address").toLowerCase();
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
        : node.kind === "explorer_target"
        ? "#f5c76e"
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
