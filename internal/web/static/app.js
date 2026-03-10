const CHAIN_LOGO_URLS = {
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

const DEFAULT_FLOW_TYPES = ["liquidity", "swaps", "bonds", "transfers"];
const GRAPH_FILTER_TXN_TYPES = [
  { key: "bond_unbond", label: "Bond/Unbond" },
  { key: "rebond", label: "Rebond" },
  { key: "transfer", label: "Send/Transfer" },
  { key: "swap", label: "Swap" },
];

const state = {
  actors: [],
  actorGraph: null,
  baseActorGraph: null,
  actorGraphRequest: null,
  actorGraphFilters: null,
  cy: null,
  expandedActors: new Set(),
  expandedExternalChains: new Set(),
  expandedHopAddressMap: new Map(),
  expansionInFlight: false,
  viewport: null,
  annotations: [],
  blocklist: [],
  graphRuns: [],
  selectedGraphRunID: null,
  explorerRuns: [],
  selectedExplorerRunID: null,
  explorerPreview: null,
  explorerGraphFilters: null,
};

const FRONTEND_LOG_PREFIX = "[chain-analysis-ui]";
let frontendLogSeq = 0;
let apiRequestSeq = 0;

function serializeError(err) {
  if (!err) {
    return null;
  }
  if (err instanceof Error) {
    return {
      name: err.name,
      message: err.message,
      stack: err.stack || "",
    };
  }
  return { message: String(err) };
}

function frontendLog(level, message, details) {
  const stamp = new Date().toISOString();
  const seq = ++frontendLogSeq;
  const line = `${FRONTEND_LOG_PREFIX} ${stamp} #${seq} [${level}] ${message}`;
  if (level === "error") {
    if (details !== undefined) {
      console.error(line, details);
    } else {
      console.error(line);
    }
    return;
  }
  if (level === "warn") {
    if (details !== undefined) {
      console.warn(line, details);
    } else {
      console.warn(line);
    }
    return;
  }
  if (details !== undefined) {
    console.log(line, details);
  } else {
    console.log(line);
  }
}

window.addEventListener("error", (event) => {
  frontendLog("error", "window_error", {
    message: event.message || "",
    source: event.filename || "",
    line: event.lineno || 0,
    column: event.colno || 0,
    error: serializeError(event.error),
  });
});

window.addEventListener("unhandledrejection", (event) => {
  frontendLog("error", "window_unhandledrejection", {
    reason: serializeError(event.reason),
  });
});

frontendLog("info", "frontend_boot", {
  path: window.location.pathname,
  user_agent: navigator.userAgent,
});

async function callAPI(path, options = {}) {
  const requestID = ++apiRequestSeq;
  const next = { ...options };
  if (next.body && typeof next.body !== "string") {
    next.headers = {
      "Content-Type": "application/json",
      ...(next.headers || {}),
    };
    next.body = JSON.stringify(next.body);
  }

  const method = String(next.method || "GET").toUpperCase();
  const startedAt = Date.now();
  frontendLog("info", `api_request_started ${method} ${path}`, { request_id: requestID, method, path });
  let response;
  try {
    response = await fetch(path, next);
  } catch (err) {
    frontendLog("error", `api_request_transport_error ${method} ${path}`, {
      request_id: requestID,
      method,
      path,
      elapsed_ms: Date.now() - startedAt,
      error: serializeError(err),
    });
    throw err;
  }
  const body = await response.json().catch((err) => {
    frontendLog("warn", `api_response_json_parse_failed ${method} ${path} status=${response.status}`, {
      request_id: requestID,
      method,
      path,
      status: response.status,
      error: serializeError(err),
    });
    return {};
  });
  const elapsedMS = Date.now() - startedAt;
  if (!response.ok) {
    frontendLog("error", `api_request_failed ${method} ${path} status=${response.status}`, {
      request_id: requestID,
      method,
      path,
      status: response.status,
      elapsed_ms: elapsedMS,
      body,
    });
    throw new Error(body.error || `Request failed: ${response.status}`);
  }
  frontendLog("info", `api_request_completed ${method} ${path} status=${response.status}`, {
    request_id: requestID,
    method,
    path,
    status: response.status,
    elapsed_ms: elapsedMS,
  });
  return body;
}

async function refreshSharedAnnotations() {
  try {
    const [annotations, blocklist] = await Promise.all([
      callAPI("/api/address-annotations"),
      callAPI("/api/address-blocklist"),
    ]);
    state.annotations = annotations.annotations || [];
    state.blocklist = blocklist.addresses || [];
  } catch {
    // silently ignore on first load if endpoints don't exist yet
  }
}

function print(el, value) {
  el.textContent = JSON.stringify(value, null, 2);
}

function ensureElements(scope, elements) {
  const missing = Object.entries(elements)
    .filter(([, value]) => !value)
    .map(([name]) => name);
  if (!missing.length) {
    return true;
  }
  frontendLog("warn", "missing_required_dom_elements", { scope, missing });
  return false;
}

function normalizeExplorerRequest(rawRequest) {
  const request = rawRequest && typeof rawRequest === "object" ? rawRequest : {};
  const direction = String(request.direction || "").trim().toLowerCase();
  const mode = String(request.mode || "").trim().toLowerCase();
  const offset = Number(request.offset);
  const batchSize = Number(request.batch_size);
  const minUSD = Number(request.min_usd);
  return {
    ...request,
    address: String(request.address || "").trim(),
    flow_types: [...DEFAULT_FLOW_TYPES],
    min_usd: Number.isFinite(minUSD) ? minUSD : 0,
    mode: mode === "preview" ? "preview" : "graph",
    direction: direction === "oldest" ? "oldest" : direction === "newest" ? "newest" : "",
    offset: Number.isFinite(offset) && offset > 0 ? Math.floor(offset) : 0,
    batch_size: Number.isFinite(batchSize) && batchSize > 0 ? Math.floor(batchSize) : 10,
  };
}

function createGraphFilterState() {
  const txnTypes = {};
  GRAPH_FILTER_TXN_TYPES.forEach((item) => {
    txnTypes[item.key] = true;
  });
  return {
    initialized: false,
    isOpen: false,
    txnTypes,
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

function normalizeISODateTime(value) {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  const date = new Date(raw);
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}

function graphFilterMetadataFromResponse(response) {
  const availableChains = uniqueStrings(
    (response?.nodes || [])
      .map((node) => String(node?.chain || "").trim().toUpperCase())
      .filter(Boolean)
  ).sort();
  let graphMinTime = "";
  let graphMaxTime = "";
  let graphMinTxnUSD = null;
  let graphMaxTxnUSD = null;
  (response?.edges || []).forEach((edge) => {
    normalizeEdgeTransactions(edge).forEach((tx) => {
      const when = normalizeISODateTime(tx.time);
      if (!when) {
      } else {
        if (!graphMinTime || when < graphMinTime) {
          graphMinTime = when;
        }
        if (!graphMaxTime || when > graphMaxTime) {
          graphMaxTime = when;
        }
      }
      const usdSpot = Number(tx?.usd_spot);
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

function chainSelectionsMatchAll(selectedChains, availableChains) {
  const selected = uniqueStrings((selectedChains || []).map((item) => String(item || "").trim().toUpperCase())).sort();
  const available = uniqueStrings((availableChains || []).map((item) => String(item || "").trim().toUpperCase())).sort();
  if (selected.length !== available.length) {
    return false;
  }
  return selected.every((item, index) => item === available[index]);
}

function timeSelectionsMatchFullRange(filters, graphMinTime, graphMaxTime) {
  const start = normalizeISODateTime(filters?.startTime || "");
  const end = normalizeISODateTime(filters?.endTime || "");
  return start === normalizeISODateTime(graphMinTime) && end === normalizeISODateTime(graphMaxTime);
}

function normalizeGraphFilterNumber(value) {
  const raw = String(value ?? "").trim();
  if (!raw) {
    return null;
  }
  const number = Number(raw);
  return Number.isFinite(number) ? number : null;
}

function graphFilterNumbersEqual(left, right) {
  const a = normalizeGraphFilterNumber(left);
  const b = normalizeGraphFilterNumber(right);
  if (a === null || b === null) {
    return a === b;
  }
  return Math.abs(a - b) < 1e-9;
}

function valueSelectionsMatchFullRange(filters, graphMinTxnUSD, graphMaxTxnUSD) {
  return (
    graphFilterNumbersEqual(filters?.minTxnUSD, graphMinTxnUSD) &&
    graphFilterNumbersEqual(filters?.maxTxnUSD, graphMaxTxnUSD)
  );
}

function clampGraphFilterNumber(value, minValue, maxValue) {
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

function formatGraphFilterNumber(value) {
  const normalized = normalizeGraphFilterNumber(value);
  if (normalized === null) {
    return "";
  }
  if (Number.isInteger(normalized)) {
    return String(normalized);
  }
  return String(normalized);
}

function clampISOToRange(value, minValue, maxValue) {
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

function syncGraphFilterStateWithResponse(filterState, response, options = {}) {
  if (!filterState) {
    return;
  }
  const { reset = false } = options;
  const previousChains = Array.isArray(filterState.availableChains) ? [...filterState.availableChains] : [];
  const previousMinTime = normalizeISODateTime(filterState.graphMinTime);
  const previousMaxTime = normalizeISODateTime(filterState.graphMaxTime);
  const previousMinTxnUSD = normalizeGraphFilterNumber(filterState.graphMinTxnUSD);
  const previousMaxTxnUSD = normalizeGraphFilterNumber(filterState.graphMaxTxnUSD);
  const selectedAllChains = chainSelectionsMatchAll(filterState.selectedChains, previousChains);
  const selectedFullRange = timeSelectionsMatchFullRange(filterState, previousMinTime, previousMaxTime);
  const selectedFullValueRange = valueSelectionsMatchFullRange(filterState, previousMinTxnUSD, previousMaxTxnUSD);
  const metadata = graphFilterMetadataFromResponse(response);

  filterState.availableChains = metadata.availableChains;
  filterState.graphMinTime = metadata.graphMinTime;
  filterState.graphMaxTime = metadata.graphMaxTime;
  filterState.graphMinTxnUSD = metadata.graphMinTxnUSD;
  filterState.graphMaxTxnUSD = metadata.graphMaxTxnUSD;

  if (reset || !filterState.initialized) {
    GRAPH_FILTER_TXN_TYPES.forEach((item) => {
      filterState.txnTypes[item.key] = true;
    });
    filterState.selectedChains = metadata.availableChains.slice();
    filterState.startTime = metadata.graphMinTime;
    filterState.endTime = metadata.graphMaxTime;
    filterState.minTxnUSD = metadata.graphMinTxnUSD;
    filterState.maxTxnUSD = metadata.graphMaxTxnUSD;
    filterState.initialized = true;
    return;
  }

  if (selectedAllChains) {
    filterState.selectedChains = metadata.availableChains.slice();
  } else {
    filterState.selectedChains = uniqueStrings(
      (filterState.selectedChains || []).filter((chain) => metadata.availableChains.includes(chain))
    );
  }
  if (!filterState.selectedChains.length && metadata.availableChains.length) {
    filterState.selectedChains = metadata.availableChains.slice();
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
    filterState.minTxnUSD = clampGraphFilterNumber(filterState.minTxnUSD, metadata.graphMinTxnUSD, metadata.graphMaxTxnUSD);
    filterState.maxTxnUSD = clampGraphFilterNumber(filterState.maxTxnUSD, metadata.graphMinTxnUSD, metadata.graphMaxTxnUSD);
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

function graphFiltersAreActive(filterState) {
  if (!filterState) {
    return false;
  }
  const allTxnEnabled = GRAPH_FILTER_TXN_TYPES.every((item) => filterState.txnTypes[item.key] !== false);
  const allChainsSelected = chainSelectionsMatchAll(filterState.selectedChains, filterState.availableChains);
  const fullRangeSelected = timeSelectionsMatchFullRange(filterState, filterState.graphMinTime, filterState.graphMaxTime);
  const fullValueRangeSelected = valueSelectionsMatchFullRange(filterState, filterState.graphMinTxnUSD, filterState.graphMaxTxnUSD);
  return !(allTxnEnabled && allChainsSelected && fullRangeSelected && fullValueRangeSelected);
}

function setGraphFilterDateValue(filterState, field, localValue) {
  if (!filterState) {
    return;
  }
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

function resetGraphFilters(filterState) {
  if (!filterState) {
    return;
  }
  GRAPH_FILTER_TXN_TYPES.forEach((item) => {
    filterState.txnTypes[item.key] = true;
  });
  filterState.selectedChains = [...(filterState.availableChains || [])];
  filterState.startTime = filterState.graphMinTime || "";
  filterState.endTime = filterState.graphMaxTime || "";
  filterState.minTxnUSD = filterState.graphMinTxnUSD;
  filterState.maxTxnUSD = filterState.graphMaxTxnUSD;
}

function setGraphFilterNumberValue(filterState, field, rawValue) {
  if (!filterState) {
    return;
  }
  const normalized = normalizeGraphFilterNumber(rawValue);
  if (normalized === null) {
    filterState[field] = field === "minTxnUSD" ? filterState.graphMinTxnUSD : filterState.graphMaxTxnUSD;
  } else {
    filterState[field] = clampGraphFilterNumber(normalized, filterState.graphMinTxnUSD, filterState.graphMaxTxnUSD);
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

function isRebondGraphAction(actionClass, actionKey, actionLabel) {
  if (String(actionClass || "").trim().toLowerCase() !== "bonds") {
    return false;
  }
  const key = `${String(actionKey || "").trim().toLowerCase()} ${String(actionLabel || "").trim().toLowerCase()}`;
  return key.includes("rebond");
}

function graphTxnBucket(actionClass, actionKey, actionLabel) {
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

function graphTxnTypeAllowed(actionClass, actionKey, actionLabel, filterState) {
  const bucket = graphTxnBucket(actionClass, actionKey, actionLabel);
  if (!bucket) {
    return true;
  }
  return filterState?.txnTypes?.[bucket] !== false;
}

function graphTxnBucketLabel(bucket) {
  const match = GRAPH_FILTER_TXN_TYPES.find((item) => item.key === bucket);
  return match ? match.label : "";
}

function summarizeGraphLabels(labels, maxVisible = 2) {
  const uniqueLabels = uniqueStrings((labels || []).map((label) => String(label || "").trim()).filter(Boolean));
  if (!uniqueLabels.length) {
    return "";
  }
  if (uniqueLabels.length <= maxVisible) {
    return uniqueLabels.join(" + ");
  }
  return `${uniqueLabels.slice(0, maxVisible).join(" + ")} +${uniqueLabels.length - maxVisible} more`;
}

function graphVisibleEdgeKey(rawEdge, from, to) {
  const actionClass = String(rawEdge?.action_class || "").trim().toLowerCase();
  return actionClass === "ownership" ? `${from}|${to}|ownership` : `${from}|${to}|flow`;
}

function resolveVisibleEdgeMetadata(edge) {
  const actionClasses = uniqueStrings(edge?.actionClasses || []);
  const actionKeys = uniqueStrings(edge?.actionKeys || []);
  const actionLabels = uniqueStrings(edge?.actionLabels || []);
  const actionDomains = uniqueStrings(edge?.actionDomains || []);
  const txnBuckets = uniqueStrings(edge?.txnBuckets || []);
  const validatorAddresses = uniqueStrings(edge?.validatorAddresses || []);
  const validatorLabels = uniqueStrings(edge?.validatorLabels || []);
  const contractTypes = uniqueStrings(edge?.contractTypes || []);
  const contractProtocols = uniqueStrings(edge?.contractProtocols || []);

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

function formatVisibleEdgeActionLabel(edge) {
  if (String(edge?.actionClass || "").trim().toLowerCase() === "ownership") {
    return "";
  }

  let label = String(edge?.actionLabel || "").trim() || "Transactions";
  const txCount = Math.max(Number(edge?.txCount || 0), Array.isArray(edge?.transactions) ? edge.transactions.length : 0);
  const mixedKinds =
    uniqueStrings(edge?.actionClasses || []).length > 1 ||
    uniqueStrings(edge?.actionLabels || []).length > 1 ||
    uniqueStrings(edge?.rawEdgeIDs || []).length > 1;
  if (txCount > 1 && mixedKinds) {
    label = `${label} (${txCount} txns)`;
  }

  const validatorAddresses = uniqueStrings(edge?.validatorAddresses || []);
  const validatorLabels = uniqueStrings(edge?.validatorLabels || []);
  const validatorCount = Math.max(validatorAddresses.length, validatorLabels.length);
  if (validatorCount === 1) {
    const validator = String(edge?.validatorLabel || edge?.validatorAddress || "").trim();
    if (validator && !label.toLowerCase().includes(" via ")) {
      label = `${label} via ${validator}`;
    }
  } else if (validatorCount > 1) {
    label = `${label} via ${validatorCount} validators`;
  }

  return label;
}

function cloneFlowAssetValue(asset) {
  return {
    asset: String(asset?.asset || ""),
    amount_raw: String(asset?.amount_raw || "0"),
    usd_spot: Number(asset?.usd_spot || 0),
    direction: String(asset?.direction || "").toLowerCase(),
    asset_kind: String(asset?.asset_kind || ""),
    token_standard: String(asset?.token_standard || ""),
    token_address: String(asset?.token_address || ""),
    token_symbol: String(asset?.token_symbol || ""),
    token_name: String(asset?.token_name || ""),
    token_decimals: Number(asset?.token_decimals || 0),
  };
}

function mergeFlowAssetValues(targetAssets, incomingAssets) {
  const assetMap = new Map();
  (targetAssets || []).forEach((asset) => {
    const cloned = cloneFlowAssetValue(asset);
    assetMap.set(`${cloned.asset}|${cloned.direction}`, cloned);
  });
  (incomingAssets || []).forEach((asset) => {
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
  return Array.from(assetMap.values()).sort((a, b) => Number(b.usd_spot || 0) - Number(a.usd_spot || 0));
}

function edgeTransactionKey(tx, index = 0) {
  const txID = String(tx?.tx_id || "").trim();
  if (txID) {
    return txID;
  }
  return `${String(tx?.height || 0)}|${normalizeISODateTime(tx?.time)}|${index}`;
}

function cloneEdgeTransaction(tx, index = 0) {
  return {
    tx_id: String(tx?.tx_id || edgeTransactionKey(tx, index)),
    height: Number(tx?.height || 0),
    time: normalizeISODateTime(tx?.time),
    usd_spot: Number(tx?.usd_spot || 0),
    assets: mergeFlowAssetValues([], tx?.assets || []),
  };
}

function normalizeEdgeTransactions(edge) {
  if (!Array.isArray(edge?.transactions) || !edge.transactions.length) {
    if (!edge) {
      return [];
    }
    return [{
      tx_id: String((edge.tx_ids || [])[0] || edge.id || ""),
      height: Number((edge.heights || [])[0] || 0),
      time: "",
      usd_spot: Number(edge.usd_spot || 0),
      assets: mergeFlowAssetValues([], edge.assets || []),
    }];
  }
  return edge.transactions.map((tx, index) => cloneEdgeTransaction(tx, index));
}

function mergeEdgeTransactions(existingTransactions, incomingTransactions) {
  const txMap = new Map();
  (existingTransactions || []).forEach((tx, index) => {
    const cloned = cloneEdgeTransaction(tx, index);
    txMap.set(edgeTransactionKey(cloned, index), cloned);
  });
  (incomingTransactions || []).forEach((tx, index) => {
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
  return Array.from(txMap.values()).sort((a, b) => {
    if (a.time === b.time) {
      return String(a.tx_id || "").localeCompare(String(b.tx_id || ""));
    }
    if (!a.time) return 1;
    if (!b.time) return -1;
    return a.time.localeCompare(b.time);
  });
}

function summarizeTransactions(transactions) {
  const txIDs = [];
  const heights = [];
  let usdSpot = 0;
  let assets = [];
  (transactions || []).forEach((tx) => {
    usdSpot += Number(tx?.usd_spot || 0);
    const txID = String(tx?.tx_id || "").trim();
    if (txID) {
      txIDs.push(txID);
    }
    const height = Number(tx?.height || 0);
    if (Number.isFinite(height) && height > 0) {
      heights.push(height);
    }
    assets = mergeFlowAssetValues(assets, tx?.assets || []);
  });
  return {
    usd_spot: usdSpot,
    tx_ids: uniqueStrings(txIDs).sort(),
    heights: uniqueNumbers(heights).sort((a, b) => a - b),
    assets,
  };
}

function filterTransactionsByTime(transactions, filterState) {
  const startTime = normalizeISODateTime(filterState?.startTime);
  const endTime = normalizeISODateTime(filterState?.endTime);
  const minTxnUSD = normalizeGraphFilterNumber(filterState?.minTxnUSD);
  const maxTxnUSD = normalizeGraphFilterNumber(filterState?.maxTxnUSD);
  return (transactions || []).filter((tx) => {
    const when = normalizeISODateTime(tx?.time);
    if ((startTime || endTime) && !when) {
      return false;
    }
    if (startTime && when < startTime) {
      return false;
    }
    if (endTime && when > endTime) {
      return false;
    }
    const usdSpot = Number(tx?.usd_spot);
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

function rawNodeChain(node) {
  return String(node?.chain || "").trim().toUpperCase();
}

function graphItemChainSet(sourceNode, targetNode) {
  return uniqueStrings([rawNodeChain(sourceNode), rawNodeChain(targetNode)].filter(Boolean));
}

function graphChainsAllowed(chainSet, filterState) {
  const selected = new Set((filterState?.selectedChains || []).map((chain) => String(chain || "").trim().toUpperCase()));
  if (!selected.size) {
    return !(filterState?.availableChains || []).length;
  }
  if (!chainSet.length) {
    return true;
  }
  return chainSet.some((chain) => selected.has(chain));
}

function filterSupportingActions(actions, response, filterState) {
  const rawNodeByID = new Map((response?.nodes || []).map((node) => [String(node.id), node]));
  const startTime = normalizeISODateTime(filterState?.startTime);
  const endTime = normalizeISODateTime(filterState?.endTime);
  const minTxnUSD = normalizeGraphFilterNumber(filterState?.minTxnUSD);
  const maxTxnUSD = normalizeGraphFilterNumber(filterState?.maxTxnUSD);
  return (actions || []).filter((action) => {
    if (!graphTxnTypeAllowed(action.action_class, action.action_key, action.action_label, filterState)) {
      return false;
    }
    const chainSet = graphItemChainSet(rawNodeByID.get(String(action.from_node || "")), rawNodeByID.get(String(action.to_node || "")));
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
    const usdSpot = Number(action?.usd_spot);
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

function graphFilterPopoverMarkup(filterState) {
  const txnOptions = GRAPH_FILTER_TXN_TYPES.map(
    (item) => `
      <label class="graph-filter-option">
        <input type="checkbox" data-filter-txn="${escapeHTML(item.key)}" ${filterState?.txnTypes?.[item.key] !== false ? "checked" : ""} />
        <span>${escapeHTML(item.label)}</span>
      </label>
    `
  ).join("");
  const chainOptions = (filterState?.availableChains || []).length
    ? filterState.availableChains
        .map(
          (chain) => `
            <label class="graph-filter-option">
              <input type="checkbox" data-filter-chain="${escapeHTML(chain)}" ${(filterState?.selectedChains || []).includes(chain) ? "checked" : ""} />
              <span>${escapeHTML(chain)}</span>
            </label>
          `
        )
        .join("")
    : `<div class="graph-filter-empty">No chains loaded.</div>`;
  const startValue = filterState?.startTime ? toLocalInputValue(new Date(filterState.startTime)) : "";
  const endValue = filterState?.endTime ? toLocalInputValue(new Date(filterState.endTime)) : "";
  const minTxnUSDValue = formatGraphFilterNumber(filterState?.minTxnUSD);
  const maxTxnUSDValue = formatGraphFilterNumber(filterState?.maxTxnUSD);
  return `
    <div class="graph-filter-head">
      <strong>Filters</strong>
      <button type="button" class="secondary" data-filter-reset>Reset</button>
    </div>
    <div class="graph-filter-section">
      <div class="graph-filter-section-title">Txn Types</div>
      <div class="graph-filter-options">${txnOptions}</div>
    </div>
    <div class="graph-filter-section">
      <div class="graph-filter-section-title">Chains Shown</div>
      <div class="graph-filter-options graph-filter-options-scroll">${chainOptions}</div>
    </div>
    <div class="graph-filter-section">
      <div class="graph-filter-section-title">Time Window</div>
      <label class="graph-filter-field">
        <span>Start</span>
        <input type="datetime-local" data-filter-time="start" value="${escapeHTML(startValue)}" />
      </label>
      <label class="graph-filter-field">
        <span>End</span>
        <input type="datetime-local" data-filter-time="end" value="${escapeHTML(endValue)}" />
      </label>
    </div>
    <div class="graph-filter-section">
      <div class="graph-filter-section-title">Txn Value ($)</div>
      <label class="graph-filter-field">
        <span>Min</span>
        <input type="number" min="0" step="any" data-filter-usd="min" value="${escapeHTML(minTxnUSDValue)}" />
      </label>
      <label class="graph-filter-field">
        <span>Max</span>
        <input type="number" min="0" step="any" data-filter-usd="max" value="${escapeHTML(maxTxnUSDValue)}" />
      </label>
    </div>
  `;
}

function updateGraphFilterButtonState(toolbar, filterState) {
  const button = toolbar?.querySelector('[data-graph-action="filters"]');
  if (!button) {
    return;
  }
  button.classList.toggle("is-active", Boolean(filterState?.isOpen) || graphFiltersAreActive(filterState));
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function ensureGraphSelectionBox(container) {
  let box = container.querySelector(".graph-selection-box");
  if (box) {
    return box;
  }
  box = document.createElement("div");
  box.className = "graph-selection-box";
  box.style.display = "none";
  container.appendChild(box);
  return box;
}

function hideGraphSelectionBox(container) {
  const box = container.querySelector(".graph-selection-box");
  if (box) {
    box.style.display = "none";
  }
}

function graphTapSuppressed(container) {
  return Date.now() < Number(container.__graphSuppressTapUntil || 0);
}

function suppressGraphTap(container, durationMS = 160) {
  container.__graphSuppressTapUntil = Date.now() + durationMS;
}

function attachGraphPointerControls({ container, getCy, hitNodeAtClientPoint }) {
  if (!container || container.__graphPointerControlsAttached) {
    return;
  }
  container.__graphPointerControlsAttached = true;

  let middlePanning = false;
  let panStart = { x: 0, y: 0 };
  let panOrigin = { x: 0, y: 0 };
  let boxSelecting = false;
  let boxDragged = false;
  let preserveSelection = false;
  let boxStart = { x: 0, y: 0 };
  let boxCurrent = { x: 0, y: 0 };

  function renderedSelectionRect() {
    const rect = container.getBoundingClientRect();
    const left = clamp(Math.min(boxStart.x, boxCurrent.x) - rect.left, 0, rect.width);
    const top = clamp(Math.min(boxStart.y, boxCurrent.y) - rect.top, 0, rect.height);
    const right = clamp(Math.max(boxStart.x, boxCurrent.x) - rect.left, 0, rect.width);
    const bottom = clamp(Math.max(boxStart.y, boxCurrent.y) - rect.top, 0, rect.height);
    return {
      x1: left,
      y1: top,
      x2: right,
      y2: bottom,
      width: Math.max(0, right - left),
      height: Math.max(0, bottom - top),
    };
  }

  function updateSelectionBox() {
    const box = ensureGraphSelectionBox(container);
    const rect = renderedSelectionRect();
    box.style.display = boxDragged ? "block" : "none";
    box.style.left = `${rect.x1}px`;
    box.style.top = `${rect.y1}px`;
    box.style.width = `${rect.width}px`;
    box.style.height = `${rect.height}px`;
  }

  function nodeIntersectsSelection(node, rect) {
    if (!node || typeof node.renderedBoundingBox !== "function") {
      return false;
    }
    const box = node.renderedBoundingBox({
      includeLabels: false,
      includeOverlays: false,
    });
    if (!box) {
      return false;
    }
    return rect.x1 <= box.x2 && rect.x2 >= box.x1 && rect.y1 <= box.y2 && rect.y2 >= box.y1;
  }

  function applyBoxSelection() {
    const cy = getCy();
    if (!cy) {
      return;
    }
    const rect = renderedSelectionRect();
    cy.batch(() => {
      if (!preserveSelection) {
        cy.elements(":selected").unselect();
      }
      cy.nodes().forEach((node) => {
        if (typeof node.visible === "function" && !node.visible()) {
          return;
        }
        if (nodeIntersectsSelection(node, rect)) {
          node.select();
        }
      });
    });
  }

  function stopBoxSelection() {
    boxSelecting = false;
    boxDragged = false;
    hideGraphSelectionBox(container);
  }

  container.addEventListener("mousedown", (e) => {
    const cy = getCy();
    if (!cy) {
      return;
    }
    if (e.button === 1) {
      middlePanning = true;
      panStart = { x: e.clientX, y: e.clientY };
      panOrigin = { ...cy.pan() };
      e.preventDefault();
      return;
    }
    if (e.button !== 0) {
      return;
    }
    if (typeof hitNodeAtClientPoint === "function" && hitNodeAtClientPoint(e.clientX, e.clientY)) {
      return;
    }
    boxSelecting = true;
    boxDragged = false;
    preserveSelection = e.shiftKey || e.metaKey || e.ctrlKey;
    boxStart = { x: e.clientX, y: e.clientY };
    boxCurrent = { ...boxStart };
    hideGraphSelectionBox(container);
    e.preventDefault();
  });

  container.addEventListener("mousemove", (e) => {
    const cy = getCy();
    if (!cy) {
      return;
    }
    if (middlePanning) {
      cy.pan({
        x: panOrigin.x + (e.clientX - panStart.x),
        y: panOrigin.y + (e.clientY - panStart.y),
      });
      return;
    }
    if (!boxSelecting) {
      return;
    }
    boxCurrent = { x: e.clientX, y: e.clientY };
    if (!boxDragged) {
      const moved = Math.abs(boxCurrent.x - boxStart.x) + Math.abs(boxCurrent.y - boxStart.y);
      if (moved < 6) {
        return;
      }
      boxDragged = true;
    }
    updateSelectionBox();
    applyBoxSelection();
    e.preventDefault();
  });

  container.addEventListener("mouseup", (e) => {
    if (e.button === 1) {
      middlePanning = false;
    }
    if (e.button !== 0 || !boxSelecting) {
      return;
    }
    if (boxDragged) {
      applyBoxSelection();
      suppressGraphTap(container);
      e.preventDefault();
      e.stopPropagation();
    }
    stopBoxSelection();
  });

  container.addEventListener("mouseleave", () => {
    middlePanning = false;
    stopBoxSelection();
  });

  container.addEventListener("auxclick", (e) => {
    if (e.button === 1) {
      e.preventDefault();
    }
  });

  container.addEventListener("wheel", (e) => {
    const cy = getCy();
    if (!cy) {
      return;
    }
    const rect = container.getBoundingClientRect();
    const renderedPosition = {
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
    };
    const currentZoom = Number(cy.zoom() || 1);
    const zoomFactor = Math.exp(-e.deltaY * 0.0015);
    const minZoom = typeof cy.minZoom === "function" ? Number(cy.minZoom() || 0.05) : 0.05;
    const maxZoom = typeof cy.maxZoom === "function" ? Number(cy.maxZoom() || 10) : 10;
    cy.zoom({
      level: clamp(currentZoom * zoomFactor, minZoom, maxZoom),
      renderedPosition,
    });
    e.preventDefault();
  }, { passive: false });
}

function bindTabs() {
  const tabs = Array.from(document.querySelectorAll(".tab"));
  const panels = Array.from(document.querySelectorAll(".tab-panel"));

  function activate(tabName) {
    tabs.forEach((tab) => tab.classList.toggle("active", tab.dataset.tab === tabName));
    panels.forEach((panel) => panel.classList.toggle("active", panel.dataset.panel === tabName));
  }

  tabs.forEach((tab) => {
    tab.addEventListener("click", () => activate(tab.dataset.tab));
  });

  return activate;
}

function bindHealth() {
  const pill = document.getElementById("health-pill");
  if (!pill) {
    frontendLog("warn", "missing_required_dom_elements", { scope: "bindHealth", missing: ["health-pill"] });
    return;
  }

  async function refresh() {
    try {
      const data = await callAPI("/api/health");
      pill.textContent = `Healthy · max ingested ${data.max_ingested}`;
    } catch (err) {
      pill.textContent = `Health check failed · ${String(err)}`;
    }
  }

  refresh();
}

function bindActorTracker(activateTab, actionLookup) {
  const form = document.getElementById("actor-form");
  const actorID = document.getElementById("actor-id");
  const actorName = document.getElementById("actor-name");
  const actorColor = document.getElementById("actor-color");
  const actorNotes = document.getElementById("actor-notes");
  const actorAddresses = document.getElementById("actor-addresses");
  const actorReset = document.getElementById("actor-reset");
  const actorList = document.getElementById("actor-list");
  const actorCount = document.getElementById("actor-count");
  const actorStatus = document.getElementById("actor-form-status");

  const actorSelection = document.getElementById("actor-selection");
  const graphForm = document.getElementById("actor-graph-form");
  const graphSummary = document.getElementById("actor-query-summary");
  const graphWarnings = document.getElementById("actor-warnings");
  const graphStats = document.getElementById("actor-graph-stats");
  const inspector = document.getElementById("actor-inspector");
  const actionsBody = document.getElementById("actor-actions-body");
  const startTime = document.getElementById("actor-start-time");
  const endTime = document.getElementById("actor-end-time");
  const maxHops = document.getElementById("actor-max-hops");
  const minUSD = document.getElementById("actor-min-usd");
  const graphContainer = document.getElementById("actor-graph");
  const contextMenu = document.getElementById("graph-context-menu");
  const graphRunList = document.getElementById("graph-run-list");
  const graphRunCount = document.getElementById("graph-run-count");
  const graphToolbar = document.getElementById("graph-toolbar");
  const graphFilterPopover = document.getElementById("graph-filter-popover");
  const graphCard = graphContainer ? graphContainer.closest(".graph-card") : null;
  if (
    !ensureElements("bindActorTracker", {
      form,
      actorID,
      actorName,
      actorColor,
      actorNotes,
      actorAddresses,
      actorReset,
      actorList,
      actorCount,
      actorStatus,
      actorSelection,
      graphForm,
      graphSummary,
      graphWarnings,
      graphStats,
      inspector,
      actionsBody,
      startTime,
      endTime,
      maxHops,
      minUSD,
      graphContainer,
      contextMenu,
      graphFilterPopover,
    })
  ) {
    return;
  }
  const actorSaveButton = form.querySelector("button[type='submit']");
  state.actorGraphFilters = createGraphFilterState();
  graphContainer.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    if (graphNodeAtClientPoint(e.clientX, e.clientY)) {
      return;
    }
    showPaneContextMenu({ x: e.clientX, y: e.clientY });
  });
  attachGraphPointerControls({
    container: graphContainer,
    getCy: () => state.cy,
    hitNodeAtClientPoint: (clientX, clientY) => graphNodeAtClientPoint(clientX, clientY),
  });
  let nodeTapTimer = null;
  let lastTappedNodeID = "";
  let lastTappedAt = 0;
  let lastPaneContextMenuOpenedAt = 0;
  let graphLabelLayer = null;
  let graphLabelFrame = 0;
  const nodeDoubleTapWindowMS = 320;

  startTime.value = toLocalInputValue(new Date(Date.now() - 7 * 24 * 60 * 60 * 1000));
  endTime.value = toLocalInputValue(new Date());

  function renderGraphFilterPopover() {
    graphFilterPopover.innerHTML = graphFilterPopoverMarkup(state.actorGraphFilters);
    graphFilterPopover.hidden = !state.actorGraphFilters.isOpen;
    updateGraphFilterButtonState(graphToolbar, state.actorGraphFilters);
  }

  function hideGraphFilterPopover() {
    if (!state.actorGraphFilters.isOpen) {
      updateGraphFilterButtonState(graphToolbar, state.actorGraphFilters);
      return;
    }
    state.actorGraphFilters.isOpen = false;
    renderGraphFilterPopover();
  }

  function toggleGraphFilterPopover() {
    if (!state.actorGraph) {
      return;
    }
    state.actorGraphFilters.isOpen = !state.actorGraphFilters.isOpen;
    renderGraphFilterPopover();
  }

  graphFilterPopover.addEventListener("change", (e) => {
    const target = e.target instanceof HTMLInputElement ? e.target : null;
    if (!target) {
      return;
    }
    if (target.dataset.filterTxn) {
      state.actorGraphFilters.txnTypes[target.dataset.filterTxn] = target.checked;
      renderGraphFilterPopover();
      renderGraphResponse();
      return;
    }
    if (target.dataset.filterChain) {
      const chain = String(target.dataset.filterChain || "");
      const selected = new Set(state.actorGraphFilters.selectedChains || []);
      if (target.checked) {
        selected.add(chain);
      } else {
        selected.delete(chain);
      }
      state.actorGraphFilters.selectedChains = Array.from(selected).sort();
      renderGraphFilterPopover();
      renderGraphResponse();
      return;
    }
    if (target.dataset.filterTime === "start") {
      setGraphFilterDateValue(state.actorGraphFilters, "startTime", target.value);
      renderGraphFilterPopover();
      renderGraphResponse();
      return;
    }
    if (target.dataset.filterTime === "end") {
      setGraphFilterDateValue(state.actorGraphFilters, "endTime", target.value);
      renderGraphFilterPopover();
      renderGraphResponse();
      return;
    }
    if (target.dataset.filterUsd === "min") {
      setGraphFilterNumberValue(state.actorGraphFilters, "minTxnUSD", target.value);
      renderGraphFilterPopover();
      renderGraphResponse();
      return;
    }
    if (target.dataset.filterUsd === "max") {
      setGraphFilterNumberValue(state.actorGraphFilters, "maxTxnUSD", target.value);
      renderGraphFilterPopover();
      renderGraphResponse();
    }
  });

  graphFilterPopover.addEventListener("click", (e) => {
    const target = e.target instanceof Element ? e.target.closest("[data-filter-reset]") : null;
    if (!target) {
      return;
    }
    resetGraphFilters(state.actorGraphFilters);
    renderGraphFilterPopover();
    renderGraphResponse();
  });

  graphContainer.addEventListener("click", (e) => {
    const target = e.target instanceof Element ? e.target.closest("[data-graph-reset-filters='actor']") : null;
    if (!target) {
      return;
    }
    resetGraphFilters(state.actorGraphFilters);
    renderGraphFilterPopover();
    renderGraphResponse();
  });

  document.addEventListener("mousedown", (e) => {
    if (!state.actorGraphFilters.isOpen) {
      return;
    }
    const target = e.target instanceof Node ? e.target : null;
    const filterButton = graphToolbar?.querySelector('[data-graph-action="filters"]');
    if ((target && graphFilterPopover.contains(target)) || (filterButton && target && filterButton.contains(target))) {
      return;
    }
    hideGraphFilterPopover();
  });

  function parseAddressLines(text) {
    const lines = text
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    const addresses = [];
    const errors = [];
    const seen = new Set();

    lines.forEach((line, index) => {
      const parts = line.split(",").map((part) => part.trim()).filter(Boolean);
      if (!parts.length) {
        return;
      }
      const address = parts[0];
      let chainHint = "";
      let label = "";
      if (parts.length === 2) {
        if (/^[A-Za-z0-9_.-]{2,12}$/.test(parts[1])) {
          chainHint = parts[1].toUpperCase();
        } else {
          label = parts[1];
        }
      }
      if (parts.length >= 3) {
        chainHint = parts[1].toUpperCase();
        label = parts.slice(2).join(", ");
      }
      if (seen.has(address.toLowerCase())) {
        errors.push(`Duplicate address on line ${index + 1}: ${address}`);
        return;
      }
      seen.add(address.toLowerCase());
      addresses.push({ address, chain_hint: chainHint, label });
    });

    return { addresses, errors };
  }

  function toNumericID(value) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }

  function normalizeGraphRequest(rawRequest) {
    const request = rawRequest && typeof rawRequest === "object" ? rawRequest : {};
    return {
      ...request,
      flow_types: [...DEFAULT_FLOW_TYPES],
      collapse_external: false,
    };
  }

  function normalizeActor(actor) {
    return {
      ...actor,
      id: toNumericID(actor.id),
      addresses: Array.isArray(actor.addresses) ? actor.addresses : [],
    };
  }

  function actorByID(id) {
    const wanted = toNumericID(id);
    if (wanted === null) {
      return null;
    }
    return state.actors.find((actor) => toNumericID(actor.id) === wanted) || null;
  }

  function resetActorForm() {
    actorID.value = "";
    actorName.value = "";
    actorColor.value = "#4ca3ff";
    actorNotes.value = "";
    actorAddresses.value = "";
    actorStatus.textContent = "";
    if (actorSaveButton) {
      actorSaveButton.textContent = "Save Actor";
    }
  }

  function fillActorForm(actor) {
    actorID.value = actor.id;
    actorName.value = actor.name;
    actorColor.value = actor.color || "#4ca3ff";
    actorNotes.value = actor.notes || "";
    actorAddresses.value = actor.addresses
      .map((item) => {
        const parts = [item.address];
        if (item.chain_hint) {
          parts.push(item.chain_hint);
        }
        if (item.label) {
          if (!item.chain_hint) {
            parts.push("");
          }
          parts.push(item.label);
        }
        return parts.filter((part, idx) => idx === 0 || part !== "").join(",");
      })
      .join("\n");
    actorStatus.textContent = `Editing ${actor.name}`;
    if (actorSaveButton) {
      actorSaveButton.textContent = "Update Actor";
    }
  }

  function renderActorSelection() {
    if (!state.actors.length) {
      actorSelection.innerHTML = `<div class="empty-state">No saved actors yet.</div>`;
      return;
    }

    actorSelection.innerHTML = state.actors
      .map(
        (actor) => `
          <label class="actor-selection-item">
            <input type="checkbox" data-actor-id="${actor.id}" />
            <span class="actor-color-swatch" style="background:${actor.color}"></span>
            <span>${escapeHTML(actor.name)}</span>
            <span class="badge">${actor.addresses.length}</span>
          </label>
        `
      )
      .join("");
  }

  function renderActorList() {
    actorCount.textContent = String(state.actors.length);
    if (!state.actors.length) {
      actorList.innerHTML = `<div class="empty-state">Create an actor to start graphing flows.</div>`;
      renderActorSelection();
      return;
    }

    actorList.innerHTML = state.actors
      .map(
        (actor) => `
          <article class="actor-card">
            <div class="actor-card-head">
              <div class="actor-card-meta">
                <span class="actor-color-swatch" style="background:${actor.color}"></span>
                <strong>${escapeHTML(actor.name)}</strong>
              </div>
              <span class="badge">${actor.addresses.length}</span>
            </div>
            <div class="actor-card-meta">
              <span>${actor.addresses.length} addresses</span>
              ${actor.notes ? `<span>${escapeHTML(actor.notes)}</span>` : ""}
            </div>
            <div class="actor-card-meta mono">
              ${actor.addresses.slice(0, 2).map((item) => escapeHTML(item.address)).join("<br />")}
              ${actor.addresses.length > 2 ? `<br />+${actor.addresses.length - 2} more` : ""}
            </div>
            <div class="actor-card-actions">
              <button class="secondary" data-edit-actor="${actor.id}" type="button">Edit</button>
              <button class="secondary" data-delete-actor="${actor.id}" type="button">Delete</button>
            </div>
          </article>
        `
      )
      .join("");
    renderActorSelection();
  }

  async function refreshActors() {
    const data = await callAPI("/api/actors");
    state.actors = (data.actors || []).map(normalizeActor).filter((actor) => actor.id !== null);
    renderActorList();
  }

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const parsed = parseAddressLines(actorAddresses.value);
    if (!actorName.value.trim()) {
      actorStatus.textContent = "Actor name is required.";
      return;
    }
    if (parsed.errors.length) {
      actorStatus.textContent = parsed.errors.join(" · ");
      return;
    }

    const payload = {
      name: actorName.value.trim(),
      color: actorColor.value,
      notes: actorNotes.value.trim(),
      addresses: parsed.addresses,
    };

    actorStatus.textContent = actorID.value ? "Updating actor…" : "Saving actor…";
    try {
      if (actorID.value) {
        await callAPI(`/api/actors/${encodeURIComponent(actorID.value)}`, {
          method: "PUT",
          body: payload,
        });
      } else {
        await callAPI("/api/actors", { method: "POST", body: payload });
      }
      resetActorForm();
      await refreshActors();
      actorStatus.textContent = "Actor saved.";
    } catch (err) {
      actorStatus.textContent = String(err);
    }
  });

  actorReset.addEventListener("click", () => resetActorForm());

  actorList.addEventListener("click", async (e) => {
    const target = e.target instanceof Element ? e.target : null;
    if (!target) {
      return;
    }

    const editButton = target.closest("[data-edit-actor]");
    if (editButton) {
      const actor = actorByID(editButton.dataset.editActor);
      if (actor) {
        fillActorForm(actor);
      }
      return;
    }

    const deleteButton = target.closest("[data-delete-actor]");
    if (!deleteButton) {
      return;
    }
    const id = toNumericID(deleteButton.dataset.deleteActor);
    if (id === null) {
      return;
    }
    const actor = actorByID(id);
    if (!actor || !window.confirm(`Delete actor "${actor.name}"?`)) {
      return;
    }
    try {
      await callAPI(`/api/actors/${encodeURIComponent(id)}`, { method: "DELETE" });
      if (state.actorGraph && state.actorGraph.actors.some((item) => item.id === id)) {
        state.actorGraph = null;
        state.baseActorGraph = null;
        state.actorGraphRequest = null;
        state.expandedHopAddressMap = new Map();
        state.actorGraphFilters.isOpen = false;
        renderGraphFilterPopover();
        clearGraphSurface();
      }
      await refreshActors();
    } catch (err) {
      actorStatus.textContent = String(err);
    }
  });

  graphForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const selectedActorIDs = Array.from(actorSelection.querySelectorAll("input[data-actor-id]:checked")).map((el) =>
      Number(el.dataset.actorId)
    );
    if (!selectedActorIDs.length) {
      graphWarnings.innerHTML = `<span class="warning-chip">Select at least one actor.</span>`;
      return;
    }

    const payload = normalizeGraphRequest({
      actor_ids: selectedActorIDs,
      start_time: new Date(startTime.value).toISOString(),
      end_time: new Date(endTime.value).toISOString(),
      max_hops: Number(maxHops.value) || 4,
      min_usd: Number(minUSD.value) || 0,
      display_mode: "combined",
    });

    graphSummary.innerHTML = `<span class="meta-chip">Building graph…</span>`;
    graphWarnings.innerHTML = "";
    graphStats.innerHTML = "";
    inspector.textContent = "Graph is loading…";
    actionsBody.innerHTML = "";

    try {
      const response = await callAPI("/api/actor-tracker/graph", {
        method: "POST",
        body: payload,
      });
      state.actorGraphRequest = payload;
      state.baseActorGraph = response;
      state.actorGraph = response;
      state.expandedActors = new Set();
      state.expandedExternalChains = new Set();
      state.expandedHopAddressMap = new Map();
      state.expansionInFlight = false;
      state.actorGraphFilters.isOpen = false;
      syncGraphFilterStateWithResponse(state.actorGraphFilters, response, { reset: true });
      renderGraphFilterPopover();
      renderGraphResponse();
      refreshGraphRuns();
    } catch (err) {
      graphWarnings.innerHTML = `<span class="warning-chip">${escapeHTML(String(err))}</span>`;
      graphSummary.innerHTML = "";
      graphStats.innerHTML = "";
      inspector.textContent = String(err);
      clearGraphSurface();
    }
  });

  actionsBody.addEventListener("click", async (e) => {
    const target = e.target instanceof Element ? e.target : null;
    if (!target) {
      return;
    }
    const button = target.closest("[data-open-tx]");
    if (!button) {
      return;
    }
    activateTab("queries");
    await actionLookup.lookup(button.dataset.openTx);
  });

  function clearGraphSurface() {
    if (graphLabelFrame) {
      window.cancelAnimationFrame(graphLabelFrame);
      graphLabelFrame = 0;
    }
    graphLabelLayer = null;
    graphContainer.innerHTML = "";
    if (state.cy) {
      state.cy.destroy();
      state.cy = null;
    }
    inspector.textContent = "Select a node or edge in the graph.";
  }

  function toggleFullscreen() {
    if (!graphCard) return;
    const isFullscreen = graphCard.classList.toggle("fullscreen");
    document.body.style.overflow = isFullscreen ? "hidden" : "";
    if (state.cy) {
      state.cy.resize();
      if (!isFullscreen) state.cy.fit(state.cy.elements(), 40);
      scheduleGraphLabelRender();
    }
    const fsButton = graphToolbar?.querySelector('[data-graph-action="fullscreen"]');
    if (fsButton) fsButton.title = isFullscreen ? "Exit fullscreen (Esc)" : "Fullscreen (F)";
  }

  function ensureGraphLabelLayer() {
    if (graphLabelLayer && graphLabelLayer.isConnected) {
      return graphLabelLayer;
    }
    graphLabelLayer = document.createElement("div");
    graphLabelLayer.className = "graph-label-layer";
    graphContainer.appendChild(graphLabelLayer);
    return graphLabelLayer;
  }

  function renderedNodeHeight(node) {
    if (!node) {
      return 0;
    }
    if (typeof node.renderedOuterHeight === "function") {
      return Number(node.renderedOuterHeight() || 0);
    }
    if (typeof node.renderedHeight === "function") {
      return Number(node.renderedHeight() || 0);
    }
    return 0;
  }

  function nodeContainsRenderedPoint(node, renderedX, renderedY) {
    if (!node || typeof node.renderedBoundingBox !== "function") {
      return false;
    }
    const box = node.renderedBoundingBox({
      includeLabels: false,
      includeOverlays: false,
    });
    if (!box) {
      return false;
    }
    return renderedX >= box.x1 && renderedX <= box.x2 && renderedY >= box.y1 && renderedY <= box.y2;
  }

  function graphNodeAtClientPoint(clientX, clientY) {
    if (!state.cy) {
      return null;
    }
    const rect = graphContainer.getBoundingClientRect();
    const renderedX = clientX - rect.left;
    const renderedY = clientY - rect.top;
    if (renderedX < 0 || renderedY < 0 || renderedX > rect.width || renderedY > rect.height) {
      return null;
    }
    const nodes = state.cy.nodes();
    for (let i = nodes.length - 1; i >= 0; i -= 1) {
      const node = nodes[i];
      if (typeof node.visible === "function" && !node.visible()) {
        continue;
      }
      if (nodeContainsRenderedPoint(node, renderedX, renderedY)) {
        return node;
      }
    }
    return null;
  }

  function renderGraphNodeLabels() {
    if (!state.cy) {
      graphLabelLayer = null;
      return;
    }
    const layer = ensureGraphLabelLayer();
    const width = graphContainer.clientWidth;
    const height = graphContainer.clientHeight;
    const viewportPadding = 140;
    const zoom = Number(state.cy.zoom() || 1);
    const labelScale = Math.max(0.3, Math.min(1.35, zoom));
    const labelFontPx = 11.84 * labelScale;
    const liveFontPx = 10.88 * labelScale;
    const labelMaxWidthPx = Math.max(48, Math.min(220, 150 * labelScale));
    const labelGapPx = Math.max(2, 8 * labelScale);
    const html = [];

    state.cy.nodes().forEach((node) => {
      const data = node.data();
      const displayLabel = String(data.displayLabel || "").trim();
      const liveHoldingsLabel = String(data.liveHoldingsLabel || "").trim();
      if (!displayLabel && !liveHoldingsLabel) {
        return;
      }

      const renderedPos = node.renderedPosition();
      if (!renderedPos) {
        return;
      }
      if (
        renderedPos.x < -viewportPadding ||
        renderedPos.x > width + viewportPadding ||
        renderedPos.y < -viewportPadding ||
        renderedPos.y > height + viewportPadding
      ) {
        return;
      }

      const renderedHeight = renderedNodeHeight(node);
      const topY = renderedPos.y - renderedHeight / 2 - labelGapPx;
      const bottomY = renderedPos.y + renderedHeight / 2 + labelGapPx;

      if (displayLabel) {
        html.push(
          `<div class="graph-node-text graph-node-label" style="left:${renderedPos.x.toFixed(1)}px;top:${topY.toFixed(1)}px;font-size:${labelFontPx.toFixed(2)}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(displayLabel)}</div>`
        );
      }
      if (liveHoldingsLabel) {
        const unavailableClass = data.liveHoldingsStatus === "error" ? " is-unavailable" : "";
        html.push(
          `<div class="graph-node-text graph-node-live${unavailableClass}" style="left:${renderedPos.x.toFixed(1)}px;top:${bottomY.toFixed(1)}px;font-size:${liveFontPx.toFixed(2)}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(liveHoldingsLabel)}</div>`
        );
      }
    });

    layer.innerHTML = html.join("");
  }

  function scheduleGraphLabelRender() {
    if (!state.cy) {
      graphLabelLayer = null;
      return;
    }
    if (graphLabelFrame) {
      return;
    }
    graphLabelFrame = window.requestAnimationFrame(() => {
      graphLabelFrame = 0;
      renderGraphNodeLabels();
    });
  }

  function renderGraphResponse() {
    if (!state.actorGraph) {
      state.actorGraphFilters.isOpen = false;
      renderGraphFilterPopover();
      clearGraphSurface();
      graphSummary.innerHTML = "";
      graphWarnings.innerHTML = "";
      graphStats.innerHTML = "";
      actionsBody.innerHTML = "";
      return;
    }

    syncGraphFilterStateWithResponse(state.actorGraphFilters, state.actorGraph);
    const visible = deriveVisibleGraph(state.actorGraph);
    const filteredActions = filterSupportingActions(state.actorGraph.supporting_actions || [], state.actorGraph, state.actorGraphFilters);
    renderGraphFilterPopover();
    renderQuerySummary(visible, filteredActions);
    renderWarnings();
    renderActions(filteredActions);
    renderGraph(visible);
  }

  function renderQuerySummary(visible, filteredActions) {
    const { query } = state.actorGraph;
    const start = new Date(query.start_time).toLocaleString();
    const end = new Date(query.end_time).toLocaleString();
    const summaryChips = [
      metaChip(`${state.actorGraph.actors.length} actors`),
      metaChip(`${query.max_hops} hops`),
      metaChip(`${start} → ${end}`),
      metaChip(query.coverage_satisfied ? "Full cache coverage" : "Partial cache coverage"),
      metaChip(`${query.blocks_scanned} blocks scanned`),
    ];
    if (state.expandedHopAddressMap.size > 0) {
      summaryChips.push(metaChip(`+${state.expandedHopAddressMap.size} one-hop seeds`));
    }
    if (graphFiltersAreActive(state.actorGraphFilters)) {
      summaryChips.push(metaChip("Filters active"));
    }
    graphSummary.innerHTML = summaryChips.join("");
    const filteredNodeCount = Array.isArray(visible?.nodes) ? visible.nodes.length : 0;
    const filteredEdgeCount = Array.isArray(visible?.edges) ? visible.edges.length : 0;
    const filteredActionCount = Array.isArray(filteredActions) ? filteredActions.length : 0;
    const totalNodeCount = Number(state.actorGraph.stats?.node_count || 0);
    const totalEdgeCount = Number(state.actorGraph.stats?.edge_count || 0);
    const totalActionCount = Number(state.actorGraph.stats?.supporting_action_count || 0);
    const showNodeFraction = graphFiltersAreActive(state.actorGraphFilters) || filteredNodeCount !== totalNodeCount;
    const showEdgeFraction = graphFiltersAreActive(state.actorGraphFilters) || filteredEdgeCount !== totalEdgeCount;
    const showActionFraction = graphFiltersAreActive(state.actorGraphFilters) || filteredActionCount !== totalActionCount;
    graphStats.innerHTML = [
      metaChip(showNodeFraction ? `${filteredNodeCount} / ${totalNodeCount} nodes` : `${totalNodeCount} nodes`),
      metaChip(showEdgeFraction ? `${filteredEdgeCount} / ${totalEdgeCount} edges` : `${totalEdgeCount} edges`),
      metaChip(showActionFraction ? `${filteredActionCount} / ${totalActionCount} actions` : `${totalActionCount} actions`),
    ].join("");
  }

  function renderWarnings() {
    const warnings = state.actorGraph.warnings || [];
    if (!warnings.length) {
      graphWarnings.innerHTML = "";
      return;
    }
    graphWarnings.innerHTML = warnings
      .map((warning) => `<span class="warning-chip">${escapeHTML(warning)}</span>`)
      .join("");
  }

  function renderActions(actions) {
    if (!actions.length) {
      actionsBody.innerHTML = `<tr><td colspan="6" class="empty-state">No supporting actions returned.</td></tr>`;
      return;
    }
    actionsBody.innerHTML = actions
      .map(
        (action) => `
          <tr>
            <td>${escapeHTML(formatDateTime(action.time))}</td>
            <td>${escapeHTML(action.action_label || action.action_class || "")}</td>
            <td>
              <button class="secondary mono" data-open-tx="${escapeHTML(action.tx_id)}" type="button">
                ${escapeHTML(shortHash(action.tx_id))}
              </button>
            </td>
            <td class="mono">${escapeHTML(action.primary_asset || "")}</td>
            <td class="mono">${escapeHTML(action.amount_raw || "")}</td>
            <td>${formatUSD(action.usd_spot)}</td>
          </tr>
        `
      )
      .join("");
  }

  function nodeAddress(nodeData) {
    const addr = String((nodeData.metrics && nodeData.metrics.address) || "").trim();
    if (addr) return addr;
    const seeds = addressesForNodeExpansion(nodeData);
    return seeds.length === 1 ? seeds[0].address : "";
  }

  function rawNodesForVisibleNode(nodeData) {
    if (!state.actorGraph) {
      return [];
    }
    const rawNodeByID = new Map((state.actorGraph.nodes || []).map((node) => [String(node.id), node]));
    const requestedIDs =
      Array.isArray(nodeData.rawNodeIDs) && nodeData.rawNodeIDs.length ? nodeData.rawNodeIDs.map((id) => String(id)) : [String(nodeData.id || "")];
    const seen = new Set();
    const out = [];
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

  function applyRefreshedLiveHoldings(graph, refreshedNodeByID) {
    if (!graph || !Array.isArray(graph.nodes) || !(refreshedNodeByID instanceof Map) || !refreshedNodeByID.size) {
      return;
    }
    graph.nodes = graph.nodes.map((node) => {
      const refreshed = refreshedNodeByID.get(String(node.id));
      if (!refreshed) {
        return node;
      }
      return {
        ...node,
        metrics: {
          ...(node.metrics || {}),
          ...(refreshed.metrics || {}),
        },
      };
    });
  }

  async function refreshLiveValueForNode(nodeData) {
    if (!state.actorGraph) {
      inspector.textContent = "Build a graph before refreshing live values.";
      return;
    }

    const rawNodes = rawNodesForVisibleNode(nodeData);
    if (!rawNodes.length) {
      inspector.textContent = "Selected node has no live value context.";
      return;
    }

    if (state.cy) {
      state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
    }
    inspector.textContent = `Refreshing live value for ${rawNodes.length} node(s)…`;

    try {
      const response = await callAPI("/api/actor-tracker/live-holdings", {
        method: "POST",
        body: { nodes: rawNodes },
      });
      const refreshedNodes = Array.isArray(response.nodes) ? response.nodes : [];
      const refreshedNodeByID = new Map(refreshedNodes.map((node) => [String(node.id), node]));
      applyRefreshedLiveHoldings(state.baseActorGraph, refreshedNodeByID);
      applyRefreshedLiveHoldings(state.actorGraph, refreshedNodeByID);

      const warnings = uniqueStrings(response.warnings || []);
      if (warnings.length) {
        state.actorGraph.warnings = uniqueStrings((state.actorGraph.warnings || []).concat(warnings));
        if (state.baseActorGraph && state.baseActorGraph !== state.actorGraph) {
          state.baseActorGraph.warnings = uniqueStrings((state.baseActorGraph.warnings || []).concat(warnings));
        }
      }

      renderGraphResponse();
      inspector.textContent = warnings.length
        ? `Refreshed live value for ${refreshedNodes.length} raw node(s). ${warnings.join(" · ")}`
        : `Refreshed live value for ${refreshedNodes.length} raw node(s).`;
    } catch (err) {
      inspector.textContent = `Live value refresh failed: ${String(err)}`;
    }
  }

  function unavailableRawNodes() {
    if (!state.actorGraph || !Array.isArray(state.actorGraph.nodes)) {
      return [];
    }
    const unavailableStatuses = new Set(["error", "unavailable"]);
    const seen = new Set();
    const out = [];
    state.actorGraph.nodes.forEach((node) => {
      const nodeID = String(node?.id || "").trim();
      const status = String(node?.metrics?.live_holdings_status || "").trim().toLowerCase();
      if (!nodeID || seen.has(nodeID) || !unavailableStatuses.has(status)) {
        return;
      }
      seen.add(nodeID);
      out.push(node);
    });
    return out;
  }

  async function refreshLiveValueForUnavailableNodes() {
    if (!state.actorGraph) {
      inspector.textContent = "Build a graph before checking unavailable live values.";
      return;
    }

    const rawNodes = unavailableRawNodes();
    if (!rawNodes.length) {
      inspector.textContent = "No nodes currently show a live value of Unavailable.";
      return;
    }

    if (state.cy) {
      state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
    }
    inspector.textContent = `Checking live value for ${rawNodes.length} unavailable node(s)…`;

    try {
      const response = await callAPI("/api/actor-tracker/live-holdings", {
        method: "POST",
        body: { nodes: rawNodes },
      });
      const refreshedNodes = Array.isArray(response.nodes) ? response.nodes : [];
      const refreshedNodeByID = new Map(refreshedNodes.map((node) => [String(node.id), node]));
      applyRefreshedLiveHoldings(state.baseActorGraph, refreshedNodeByID);
      applyRefreshedLiveHoldings(state.actorGraph, refreshedNodeByID);

      const warnings = uniqueStrings(response.warnings || []);
      if (warnings.length) {
        state.actorGraph.warnings = uniqueStrings((state.actorGraph.warnings || []).concat(warnings));
        if (state.baseActorGraph && state.baseActorGraph !== state.actorGraph) {
          state.baseActorGraph.warnings = uniqueStrings((state.baseActorGraph.warnings || []).concat(warnings));
        }
      }

      renderGraphResponse();
      const stillUnavailable = unavailableRawNodes().length;
      const suffix = stillUnavailable
        ? ` ${stillUnavailable} node(s) still unavailable.`
        : " All unavailable nodes refreshed.";
      inspector.textContent = warnings.length
        ? `Checked ${rawNodes.length} unavailable node(s); refreshed ${refreshedNodes.length}. ${warnings.join(" · ")}${suffix}`
        : `Checked ${rawNodes.length} unavailable node(s); refreshed ${refreshedNodes.length}.${suffix}`;
    } catch (err) {
      inspector.textContent = `Unavailable live value check failed: ${String(err)}`;
    }
  }

  function addressesForNodeExpansion(nodeData) {
    if (!state.actorGraph) {
      return [];
    }
    const rawNodeByID = new Map((state.actorGraph.nodes || []).map((node) => [node.id, node]));
    const out = [];
    const rawNodeIDs = Array.isArray(nodeData.rawNodeIDs) ? nodeData.rawNodeIDs : [];
    rawNodeIDs.forEach((rawID) => {
      const rawNode = rawNodeByID.get(rawID);
      const candidate = rawNode && rawNode.metrics ? String(rawNode.metrics.address || "").trim() : "";
      if (candidate) {
        out.push(buildFrontierSeed(candidate, rawNode?.chain));
      }
    });

    if (!out.length && nodeData.kind === "actor") {
      (nodeData.actorIds || []).forEach((actorID) => {
        const actor = (state.baseActorGraph?.actors || []).find((item) => Number(item.id) === Number(actorID));
        (actor?.addresses || []).forEach((entry) => {
          const candidate = String(entry.address || "").trim();
          if (candidate) {
            out.push(buildFrontierSeed(candidate, entry.chain_hint));
          }
        });
      });
    }

    const seen = new Map();
    out.forEach((seed) => {
      if (!seed || !seed.encoded) {
        return;
      }
      if (!seen.has(seed.encoded)) {
        seen.set(seed.encoded, seed);
      }
    });
    return Array.from(seen.values());
  }

  function mergeActorGraphResponse(base, expansion) {
    if (!base) {
      return expansion;
    }

    function nodeMergeKey(node) {
      if (!node) {
        return "";
      }
      const metrics = node.metrics || {};
      const address = String(metrics.address || "").trim().toLowerCase();
      const pool = String(metrics.pool || "").trim().toUpperCase();
      const chain = String(node.chain || "").trim().toUpperCase();
      const kind = String(node.kind || "").trim().toLowerCase();
      if (address) {
        return `${kind}|${chain}|${address}`;
      }
      if (pool) {
        return `${kind}|${pool}`;
      }
      if (kind === "actor" && Array.isArray(node.actor_ids) && node.actor_ids.length === 1) {
        return `actor|${node.actor_ids[0]}`;
      }
      return String(node.id || "");
    }

    function edgeMergeKey(edge, nodeAlias) {
      const from = nodeAlias.get(edge.from) || edge.from;
      const to = nodeAlias.get(edge.to) || edge.to;
      let key = `${from}|${to}|${edge.action_key || edge.action_class}`;
      if (edge.validator_address && String(edge.action_key || edge.action_class || "").toLowerCase().includes("rebond")) {
        key += `|validator:${edge.validator_address}`;
      }
      return key;
    }

    const actorMap = new Map();
    (base.actors || []).forEach((actor) => actorMap.set(Number(actor.id), actor));
    (expansion.actors || []).forEach((actor) => actorMap.set(Number(actor.id), actor));

    const nodeMap = new Map();
    const nodeAlias = new Map();
    const nodeKeyToID = new Map();

    function mergeNode(node) {
      const mergeKey = nodeMergeKey(node);
      const existingID = nodeKeyToID.get(mergeKey) || node.id;
      nodeAlias.set(node.id, existingID);
      const existing = nodeMap.get(existingID);
      if (!existing) {
        nodeKeyToID.set(mergeKey, existingID);
        nodeMap.set(existingID, {
          ...node,
          id: existingID,
          actor_ids: [...(node.actor_ids || [])],
          metrics: { ...(node.metrics || {}) },
        });
        return;
      }
      existing.actor_ids = uniqueNumbers((existing.actor_ids || []).concat(node.actor_ids || []));
      existing.shared = Boolean(existing.shared || node.shared);
      existing.collapsed = Boolean(existing.collapsed && node.collapsed);
      existing.depth = Math.min(Number(existing.depth || 0), Number(node.depth || 0));
      existing.metrics = { ...(existing.metrics || {}), ...(node.metrics || {}) };
    }

    (base.nodes || []).forEach(mergeNode);
    (expansion.nodes || []).forEach(mergeNode);

    function cloneMergedEdge(edge, canonicalID) {
      const transactions = mergeEdgeTransactions([], normalizeEdgeTransactions(edge));
      const summary = summarizeTransactions(transactions);
      return {
        ...edge,
        id: canonicalID,
        from: nodeAlias.get(edge.from) || edge.from,
        to: nodeAlias.get(edge.to) || edge.to,
        actor_ids: [...(edge.actor_ids || [])],
        transactions,
        tx_ids: summary.tx_ids,
        heights: summary.heights,
        assets: summary.assets,
        usd_spot: summary.usd_spot,
      };
    }

    const edgeMap = new Map();
    (base.edges || []).forEach((edge) => {
      const canonicalID = edgeMergeKey(edge, nodeAlias);
      edgeMap.set(canonicalID, cloneMergedEdge(edge, canonicalID));
    });
    (expansion.edges || []).forEach((edge) => {
      const canonicalID = edgeMergeKey(edge, nodeAlias);
      const existing = edgeMap.get(canonicalID);
      if (!existing) {
        edgeMap.set(canonicalID, cloneMergedEdge(edge, canonicalID));
        return;
      }
      existing.actor_ids = uniqueNumbers((existing.actor_ids || []).concat(edge.actor_ids || []));
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
    });

    const actionKey = (action) => `${action.tx_id}|${action.action_key || action.action_class}|${action.from_node}|${action.to_node}`;
    const actionMap = new Map((base.supporting_actions || []).map((action) => [actionKey(action), action]));
    (expansion.supporting_actions || []).forEach((action) => {
      const key = actionKey(action);
      if (!actionMap.has(key)) {
        actionMap.set(key, action);
      }
    });

    const baseQuery = base.query || {};
    const expansionQuery = expansion.query || {};
    const mergedQuery = {
      ...baseQuery,
      blocks_scanned: Number(baseQuery.blocks_scanned || 0) + Number(expansionQuery.blocks_scanned || 0),
      coverage_satisfied: Boolean(baseQuery.coverage_satisfied) && Boolean(expansionQuery.coverage_satisfied),
    };

    const merged = {
      ...base,
      query: mergedQuery,
      actors: Array.from(actorMap.values()),
      warnings: uniqueStrings((base.warnings || []).concat(expansion.warnings || [])),
      nodes: Array.from(nodeMap.values()),
      edges: Array.from(edgeMap.values()),
      supporting_actions: Array.from(actionMap.values()),
    };
    merged.stats = {
      ...(base.stats || {}),
      actor_count: merged.actors.length,
      node_count: merged.nodes.length,
      edge_count: merged.edges.length,
      supporting_action_count: merged.supporting_actions.length,
      one_hop_expansion: true,
      expanded_seed_count: state.expandedHopAddressMap.size,
    };
    return merged;
  }

  async function expandOneHopFromNode(nodeData) {
    if (!state.baseActorGraph || !state.actorGraphRequest) {
      return;
    }
    if (state.expansionInFlight) {
      return;
    }

    const seeds = addressesForNodeExpansion(nodeData);
    if (!seeds.length) {
      inspector.textContent = "Selected node has no address context to expand.";
      return;
    }

    const newlyAddedKeys = [];
    seeds.forEach((seed) => {
      const key = addressKey(seed.encoded);
      if (!key || state.expandedHopAddressMap.has(key)) {
        return;
      }
      state.expandedHopAddressMap.set(key, seed.encoded);
      newlyAddedKeys.push(key);
    });
    if (!newlyAddedKeys.length) {
      inspector.textContent = "One-hop expansion already loaded for this node.";
      return;
    }

    state.expansionInFlight = true;
    if (state.cy) {
      state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
    }
    inspector.textContent = `Expanding one hop from ${seeds.length} address(es)…`;

    try {
      const base = normalizeGraphRequest(state.actorGraphRequest);
      state.actorGraphRequest = base;
      const expansion = await callAPI("/api/actor-tracker/expand", {
        method: "POST",
        body: {
          actor_ids: base.actor_ids || [],
          start_time: base.start_time,
          end_time: base.end_time,
          flow_types: base.flow_types,
          min_usd: Number(base.min_usd || 0),
          collapse_external: base.collapse_external,
          display_mode: base.display_mode || "combined",
          addresses: Array.from(state.expandedHopAddressMap.values()),
        },
      });
      state.actorGraph = mergeActorGraphResponse(state.baseActorGraph, expansion);
      renderGraphResponse();
      inspector.textContent = `Loaded one-hop expansion for ${state.expandedHopAddressMap.size} address seed(s).`;
    } catch (err) {
      newlyAddedKeys.forEach((key) => state.expandedHopAddressMap.delete(key));
      const message = `One-hop expansion failed: ${String(err)}`;
      graphWarnings.innerHTML = uniqueStrings((state.actorGraph?.warnings || []).concat(message))
        .map((warning) => `<span class="warning-chip">${escapeHTML(warning)}</span>`)
        .join("");
      inspector.textContent = message;
    } finally {
      state.expansionInFlight = false;
    }
  }

  function renderGraph(visible) {
    if (!visible.nodes.length) {
      clearGraphSurface();
      graphContainer.innerHTML = graphFiltersAreActive(state.actorGraphFilters)
        ? `<div class="empty-state">No graph elements match the current filters. <button type="button" class="secondary" data-graph-reset-filters="actor">Reset filters</button></div>`
        : `<div class="empty-state">No graphable flows found for the selected actors and time window.</div>`;
      return;
    }

    const elements = [
      ...visible.nodes.map((node) => ({ data: node })),
      ...visible.edges.map((edge) => ({ data: edge })),
    ];

    if (!state.cy) {
      graphContainer.innerHTML = "";
      state.cy = cytoscape({
        container: graphContainer,
        elements,
        style: graphStylesheet(),
        wheelSensitivity: 0.3,
        zoomingEnabled: true,
        userZoomingEnabled: false,
        boxSelectionEnabled: false,
        selectionType: "additive",
        userPanningEnabled: false,
        autoungrabify: false,
      });

      state.cy.on("tap", "node", (event) => {
        if (graphTapSuppressed(graphContainer)) {
          return;
        }
        const data = event.target.data();
        const nodeID = String(event.target.id() || "");
        const now = Date.now();

        if (nodeID && nodeID === lastTappedNodeID && now - lastTappedAt <= nodeDoubleTapWindowMS) {
          if (nodeTapTimer) {
            clearTimeout(nodeTapTimer);
            nodeTapTimer = null;
          }
          lastTappedNodeID = "";
          lastTappedAt = 0;
          void expandOneHopFromNode(data);
          return;
        }

        lastTappedNodeID = nodeID;
        lastTappedAt = now;

        if (nodeTapTimer) {
          clearTimeout(nodeTapTimer);
          nodeTapTimer = null;
        }

        nodeTapTimer = setTimeout(() => {
          nodeTapTimer = null;
          lastTappedNodeID = "";
          lastTappedAt = 0;

          if (data.kind === "actor" && data.actorIds.length === 1) {
            const actorID = data.actorIds[0];
            if (state.expandedActors.has(actorID)) {
              state.expandedActors.delete(actorID);
            } else {
              state.expandedActors.add(actorID);
            }
            state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
            renderGraphResponse();
            return;
          }
          if (data.kind === "external_cluster") {
            const chain = data.chain || "UNKNOWN";
            if (state.expandedExternalChains.has(chain)) {
              state.expandedExternalChains.delete(chain);
            } else {
              state.expandedExternalChains.add(chain);
            }
            state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
            renderGraphResponse();
            return;
          }
          inspector.textContent = JSON.stringify(data.inspect, null, 2);
        }, nodeDoubleTapWindowMS);

        return;
      });

      state.cy.on("cxttap", "node", (event) => {
        const data = event.target.data();
        const rect = graphContainer.getBoundingClientRect();
        const renderedPos = event.renderedPosition || event.target.renderedPosition();
        showContextMenu(data, {
          x: rect.left + renderedPos.x,
          y: rect.top + renderedPos.y,
        });
      });

      state.cy.on("cxttap", (event) => {
        if (event.target !== state.cy) {
          return;
        }
        if (Date.now() - lastPaneContextMenuOpenedAt < 120) {
          return;
        }
        const rect = graphContainer.getBoundingClientRect();
        const renderedPos = event.renderedPosition || {
          x: graphContainer.clientWidth / 2,
          y: graphContainer.clientHeight / 2,
        };
        showPaneContextMenu({
          x: rect.left + renderedPos.x,
          y: rect.top + renderedPos.y,
        });
      });

      state.cy.on("tap", "edge", (event) => {
        if (graphTapSuppressed(graphContainer)) {
          return;
        }
        inspector.textContent = JSON.stringify(event.target.data().inspect, null, 2);
      });

      state.cy.on("zoom pan", () => {
        state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
      });
      state.cy.on("render zoom pan resize add remove data position", () => {
        scheduleGraphLabelRender();
      });
    } else {
      state.cy.elements().remove();
      state.cy.add(elements);
    }

    applyElkLayout(state.cy, visible.nodes)
      .then(() => {
        if (state.viewport) {
          state.cy.zoom(state.viewport.zoom);
          state.cy.pan(state.viewport.pan);
        } else {
          state.cy.fit(state.cy.elements(), 40);
        }
        scheduleGraphLabelRender();
      })
      .catch((err) => {
        inspector.textContent = `Layout failed: ${String(err)}`;
      });
  }

  function mergeLiveHoldingsStatus(current, incoming) {
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

  function deriveVisibleGraph(response) {
    const rawNodes = response.nodes || [];
    const rawEdges = response.edges || [];
    const rawNodeByID = new Map(rawNodes.map((node) => [node.id, node]));
    const actorByIDMap = new Map((response.actors || []).map((actor) => [actor.id, actor]));
    const filterState = state.actorGraphFilters || createGraphFilterState();
    const incidentCounts = new Map();
    const nodeUSD = new Map();
    const filteredRawEdges = [];

    // Build sets for blocklist and asgard vault annotations
    const blocklistedAddresses = new Set(state.blocklist.map((b) => String(b.normalized_address || "").toLowerCase()));
    const asgardAddresses = new Set(
      state.annotations
        .filter((a) => a.kind === "asgard_vault")
        .map((a) => String(a.normalized_address || "").toLowerCase())
    );
    const labelAnnotations = new Map(
      state.annotations
        .filter((a) => a && a.kind === "label" && a.value !== null && a.value !== undefined)
        .map((a) => [String(a.normalized_address || "").toLowerCase(), String(a.value)])
    );
    const hiddenAddresses = new Set([...blocklistedAddresses, ...asgardAddresses]);

    function isHiddenAddress(node) {
      const addr = String((node.metrics && node.metrics.address) || "").trim().toLowerCase();
      return addr && hiddenAddresses.has(addr);
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
      [rawEdge.from, rawEdge.to].forEach((id) => {
        incidentCounts.set(id, (incidentCounts.get(id) || 0) + 1);
        nodeUSD.set(id, (nodeUSD.get(id) || 0) + Number(summary.usd_spot || 0));
      });
    });

    const visibleNodes = new Map();
    const visibleEdges = new Map();

    function lowSignalExternal(node) {
      if (!response.query.collapse_external) {
        return false;
      }
      if (node.kind !== "external_address" || (node.actor_ids || []).length) {
        return false;
      }
      if (state.expandedExternalChains.has(node.chain || "UNKNOWN")) {
        return false;
      }
      if ((incidentCounts.get(node.id) || 0) > 1) {
        return false;
      }
      const threshold = Math.max(10000, Number(response.query.min_usd || 0));
      return (nodeUSD.get(node.id) || 0) < threshold;
    }

    function mapNodeID(rawNode) {
      if (!rawNode) {
        return null;
      }
      if (isHiddenAddress(rawNode)) {
        return null;
      }
      if (rawNode.kind === "actor_address" && !rawNode.shared && rawNode.actor_ids.length === 1) {
        const ownerID = rawNode.actor_ids[0];
        const ownerActor = actorByIDMap.get(ownerID);
        const ownerAddressCount = Array.isArray(ownerActor?.addresses) ? ownerActor.addresses.length : 0;
        if (ownerAddressCount > 1) {
          return rawNode.id;
        }
        if (!state.expandedActors.has(ownerID)) {
          return `actor:${ownerID}`;
        }
      }
      if (lowSignalExternal(rawNode)) {
        return `external_cluster:${rawNode.chain || "UNKNOWN"}`;
      }
      return rawNode.id;
    }

    function ensureVisibleNode(rawNode, mappedID) {
      if (!mappedID) {
        return;
      }
      if (mappedID.startsWith("external_cluster:")) {
        const chain = mappedID.split(":")[1] || "UNKNOWN";
        const existing = visibleNodes.get(mappedID) || {
          id: mappedID,
          label: `${chain} External Cluster`,
          kind: "external_cluster",
          chain,
          stage: "external",
          depth: rawNode.depth,
          actorIds: [],
          shared: false,
          collapsed: true,
          rawNodeIDs: [],
          metrics: { address_count: 0, usd_spot: 0, live_holdings_usd_spot: 0, live_holdings_available: false, live_holdings_status: "" },
        };
        if (!existing.rawNodeIDs.includes(rawNode.id)) {
          existing.rawNodeIDs.push(rawNode.id);
          existing.metrics.address_count += 1;
          existing.metrics.usd_spot += nodeUSD.get(rawNode.id) || 0;
          existing.metrics.live_holdings_usd_spot += Number(rawNode?.metrics?.live_holdings_usd_spot || 0);
          existing.metrics.live_holdings_available =
            Boolean(existing.metrics.live_holdings_available) || Boolean(rawNode?.metrics?.live_holdings_available);
          existing.metrics.live_holdings_status = mergeLiveHoldingsStatus(
            existing.metrics.live_holdings_status,
            rawNode?.metrics?.live_holdings_status
          );
          existing.depth = Math.min(existing.depth, rawNode.depth);
        }
        visibleNodes.set(mappedID, existing);
        return;
      }

      const actor = rawNode.kind === "actor" && rawNode.actor_ids.length === 1 ? actorByIDMap.get(rawNode.actor_ids[0]) : null;
      const ownerActor = rawNode.actor_ids.length === 1 ? actorByIDMap.get(rawNode.actor_ids[0]) : null;
      const existing = visibleNodes.get(mappedID) || {
        id: mappedID,
        label: rawNode.label,
        kind: rawNode.kind,
        chain: rawNode.chain,
        stage: rawNode.stage,
        depth: rawNode.depth,
        actorIds: [...(rawNode.actor_ids || [])],
        shared: Boolean(rawNode.shared),
        collapsed: Boolean(rawNode.collapsed),
        rawNodeIDs: [],
        metrics: { ...(rawNode.metrics || {}), live_holdings_usd_spot: 0 },
        color: actor ? actor.color : ownerActor ? ownerActor.color : "#83a8dc",
      };
      if (!existing.rawNodeIDs.includes(rawNode.id)) {
        existing.rawNodeIDs.push(rawNode.id);
        existing.depth = Math.min(existing.depth, rawNode.depth);
        existing.actorIds = uniqueNumbers(existing.actorIds.concat(rawNode.actor_ids || []));
        existing.shared = existing.shared || Boolean(rawNode.shared);
        existing.metrics.live_holdings_usd_spot =
          Number(existing.metrics.live_holdings_usd_spot || 0) + Number(rawNode?.metrics?.live_holdings_usd_spot || 0);
        existing.metrics.live_holdings_available =
          Boolean(existing.metrics.live_holdings_available) || Boolean(rawNode?.metrics?.live_holdings_available);
        existing.metrics.live_holdings_status = mergeLiveHoldingsStatus(
          existing.metrics.live_holdings_status,
          rawNode?.metrics?.live_holdings_status
        );
      }
      if (rawNode.kind === "actor" && actor) {
        existing.color = actor.color;
      }
      if (!visibleNodes.has(mappedID)) {
        visibleNodes.set(mappedID, existing);
      }
    }

    function addVisibleEdge(rawEdge) {
      const sourceNode = rawNodeByID.get(rawEdge.from);
      const targetNode = rawNodeByID.get(rawEdge.to);
      const from = mapNodeID(sourceNode);
      const to = mapNodeID(targetNode);
      if (!from || !to || from === to) {
        return false;
      }
      ensureVisibleNode(sourceNode, from);
      ensureVisibleNode(targetNode, to);

      const edgeID = graphVisibleEdgeKey(rawEdge, from, to);
      const existing = visibleEdges.get(edgeID) || {
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
        actorIds: [],
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
      existing.actorIds = uniqueNumbers(existing.actorIds.concat(rawEdge.actor_ids || []));
      existing.rawEdgeIDs = uniqueStrings(existing.rawEdgeIDs.concat(rawEdge.id));
      existing.actionClasses = uniqueStrings(existing.actionClasses.concat(String(rawEdge.action_class || "").trim()).filter(Boolean));
      existing.actionKeys = uniqueStrings(existing.actionKeys.concat(String(rawEdge.action_key || rawEdge.action_class || "").trim()).filter(Boolean));
      existing.actionLabels = uniqueStrings(existing.actionLabels.concat(String(rawEdge.action_label || rawEdge.action_class || "").trim()).filter(Boolean));
      existing.actionDomains = uniqueStrings(existing.actionDomains.concat(String(rawEdge.action_domain || rawEdge.action_class || "").trim()).filter(Boolean));
      const txnBucket = graphTxnBucket(rawEdge.action_class, rawEdge.action_key, rawEdge.action_label);
      if (txnBucket) {
        existing.txnBuckets = uniqueStrings(existing.txnBuckets.concat(txnBucket));
      }
      if (rawEdge.validator_address) {
        existing.validatorAddresses = uniqueStrings(existing.validatorAddresses.concat(String(rawEdge.validator_address).trim()));
      }
      if (rawEdge.validator_label) {
        existing.validatorLabels = uniqueStrings(existing.validatorLabels.concat(String(rawEdge.validator_label).trim()));
      }
      if (rawEdge.contract_type) {
        existing.contractTypes = uniqueStrings(existing.contractTypes.concat(String(rawEdge.contract_type).trim()));
      }
      if (rawEdge.contract_protocol) {
        existing.contractProtocols = uniqueStrings(existing.contractProtocols.concat(String(rawEdge.contract_protocol).trim()));
      }
      const metadata = resolveVisibleEdgeMetadata(existing);
      existing.actionClass = metadata.actionClass;
      existing.actionKey = metadata.actionKey;
      existing.actionLabel = metadata.actionLabel;
      existing.actionDomain = metadata.actionDomain;
      existing.validatorAddress = metadata.validatorAddress;
      existing.validatorLabel = metadata.validatorLabel;
      existing.contractType = metadata.contractType;
      existing.contractProtocol = metadata.contractProtocol;
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
      existing.inspect.action_classes = metadata.actionClasses;
      existing.inspect.action_keys = metadata.actionKeys;
      existing.inspect.action_labels = metadata.actionLabels;
      existing.inspect.action_domains = metadata.actionDomains;
      existing.inspect.action_buckets = metadata.txnBuckets;
      existing.inspect.validator_addresses = metadata.validatorAddresses;
      existing.inspect.validator_labels = metadata.validatorLabels;
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
      existing.inspect.edges.push(rawEdge);
      existing.width = edgeWidth(existing.usdSpot);
      visibleEdges.set(edgeID, existing);
      return true;
    }

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

    const nodes = Array.from(visibleNodes.values()).map((node) => decorateVisibleNode(node, actorByIDMap, labelAnnotations));
    const edges = Array.from(visibleEdges.values()).map((edge) => {
      const aggregatedAssets = Object.entries(edge.assetTotals)
        .map(([, bucket]) => ({
          asset: bucket.asset,
          direction: bucket.direction || "",
          amount_raw: bucket.amountRaw,
          usd_spot: bucket.usdSpot,
          token_symbol: bucket.tokenSymbol || "",
        }))
        .sort((a, b) => Number(b.usd_spot || 0) - Number(a.usd_spot || 0));
      const tokenSummary =
        edge.actionClass === "swaps" ? formatSwapTokenSummary(aggregatedAssets) : formatEdgeTokenSummary(aggregatedAssets);
      const usdSummary = formatUSD(edge.usdSpot);
      const edgeActionLabel = formatVisibleEdgeActionLabel(edge);
      const edgeLabel =
        edge.actionClass === "ownership"
          ? ""
          : `${edgeActionLabel} · ${tokenSummary}\n${usdSummary}`;

      return {
        ...edge,
        edgeLabel,
        actorIds: uniqueNumbers(edge.actorIds),
        lineColor: edgeColor(edge.actionClass),
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

  function graphStylesheet() {
    return [
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
          "opacity": 0.82,
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
  }

  async function applyElkLayout(cy, nodes) {
    const elk = new ELK();
    const graph = {
      id: "root",
      layoutOptions: {
        "elk.algorithm": "layered",
        "elk.direction": "RIGHT",
        "elk.edgeRouting": "POLYLINE",
        "elk.layered.spacing.nodeNodeBetweenLayers": "110",
        "elk.spacing.nodeNode": "42",
        "elk.padding": "[top=32,left=32,bottom=32,right=32]",
      },
      children: nodes.map((node) => ({
        id: node.id,
        width: node.kind === "actor" ? 150 : node.kind === "pool" ? 96 : 84,
        height: node.kind === "actor" ? 64 : node.kind === "pool" ? 96 : 72,
      })),
      edges: cy
        .edges()
        .toArray()
        .map((edge) => ({
          id: edge.id(),
          sources: [edge.data("source")],
          targets: [edge.data("target")],
        })),
    };

    const result = await elk.layout(graph);
    const positions = new Map((result.children || []).map((child) => [child.id, { x: child.x, y: child.y }]));
    cy.layout({
      name: "preset",
      fit: false,
      animate: false,
      positions(node) {
        return positions.get(node.id()) || { x: node.data("depth") * 180, y: 80 };
      },
    }).run();
  }

  // --- Context Menu ---
  let contextMenuTarget = null;
  let contextMenuMode = "node";

  function updateContextMenuButtons(mode) {
    const normalizedMode = String(mode || "").trim().toLowerCase();
    const buttons = contextMenu.querySelectorAll("button[data-action]");
    buttons.forEach((button) => {
      const scopes = String(button.dataset.scope || "node,pane")
        .split(",")
        .map((value) => value.trim().toLowerCase())
        .filter(Boolean);
      const visible = scopes.length ? scopes.includes(normalizedMode) : true;
      button.style.display = visible ? "block" : "none";
    });
  }

  function placeContextMenu(pagePos) {
    contextMenu.style.display = "block";
    const menuRect = contextMenu.getBoundingClientRect();
    const x = Math.min(pagePos.x, window.innerWidth - menuRect.width - 8);
    const y = Math.min(pagePos.y, window.innerHeight - menuRect.height - 8);
    contextMenu.style.left = `${x}px`;
    contextMenu.style.top = `${y}px`;
  }

  function showContextMenu(nodeData, pagePos) {
    contextMenuTarget = nodeData;
    contextMenuMode = "node";
    updateContextMenuButtons(contextMenuMode);
    placeContextMenu(pagePos);
  }

  function showPaneContextMenu(pagePos) {
    contextMenuTarget = null;
    contextMenuMode = "pane";
    updateContextMenuButtons(contextMenuMode);
    placeContextMenu(pagePos);
    lastPaneContextMenuOpenedAt = Date.now();
  }

  function hideContextMenu() {
    contextMenu.style.display = "none";
    contextMenuTarget = null;
    contextMenuMode = "node";
  }

  document.addEventListener("click", (e) => {
    if (!(e.target instanceof Node) || !contextMenu.contains(e.target)) hideContextMenu();
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      hideContextMenu();
      hideGraphFilterPopover();
      if (graphCard && graphCard.classList.contains("fullscreen")) toggleFullscreen();
      return;
    }
    const tag = document.activeElement?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;
    if (!document.querySelector('[data-panel="actor-tracker"].active')) return;
    if (!state.cy) return;
    switch (e.key) {
      case "+":
      case "=":
        e.preventDefault();
        state.cy.zoom({ level: state.cy.zoom() * 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
        break;
      case "-":
      case "_":
        e.preventDefault();
        state.cy.zoom({ level: state.cy.zoom() / 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
        break;
      case "0":
        e.preventDefault();
        state.cy.fit(state.cy.elements(), 40);
        state.viewport = null;
        break;
      case "f":
      case "F":
        e.preventDefault();
        toggleFullscreen();
        break;
    }
  });

  if (graphToolbar) {
    graphToolbar.addEventListener("click", (e) => {
      const button = e.target instanceof Element ? e.target.closest("button[data-graph-action]") : null;
      if (!button) return;
      if (button.dataset.graphAction === "filters") {
        toggleGraphFilterPopover();
        return;
      }
      if (!state.cy) return;
      switch (button.dataset.graphAction) {
        case "zoom-in":
          state.cy.zoom({ level: state.cy.zoom() * 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
          break;
        case "zoom-out":
          state.cy.zoom({ level: state.cy.zoom() / 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
          break;
        case "fit":
          state.cy.fit(state.cy.elements(), 40);
          state.viewport = null;
          break;
        case "fullscreen":
          toggleFullscreen();
          break;
      }
    });
  }

  contextMenu.addEventListener("click", async (e) => {
    const target = e.target instanceof Element ? e.target : null;
    if (!target) {
      return;
    }
    const button = target.closest("button[data-action]");
    if (!button) return;
    const action = button.dataset.action;
    const data = contextMenuTarget;
    hideContextMenu();
    if (action === "check-unavailable") {
      await refreshLiveValueForUnavailableNodes();
      return;
    }
    if (!data) return;
    const address = nodeAddress(data);

    switch (action) {
      case "explorer": {
        const url = explorerURLForAddress(address, data.chain);
        if (url) window.open(url, "_blank");
        break;
      }
      case "copy-address":
        if (address) {
          navigator.clipboard.writeText(address).then(() => {
            inspector.textContent = `Copied: ${address}`;
          });
        }
        break;
      case "refresh-live-value":
        await refreshLiveValueForNode(data);
        break;
      case "expand-hop":
        void expandOneHopFromNode(data);
        break;
      case "label-node": {
        const label = prompt("Enter label for this node:", "");
        if (label !== null && address) {
          try {
            await callAPI("/api/address-annotations", {
              method: "PUT",
              body: { address, kind: "label", value: label },
            });
            await refreshSharedAnnotations();
            if (state.cy) state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
            renderGraphResponse();
          } catch (err) {
            inspector.textContent = `Label failed: ${String(err)}`;
          }
        }
        break;
      }
      case "mark-asgard": {
        if (address) {
          try {
            await callAPI("/api/address-annotations", {
              method: "PUT",
              body: { address, kind: "asgard_vault", value: "true" },
            });
            await refreshSharedAnnotations();
            if (state.cy) state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
            renderGraphResponse();
          } catch (err) {
            inspector.textContent = `Mark Asgard failed: ${String(err)}`;
          }
        }
        break;
      }
      case "remove-node": {
        if (address) {
          try {
            await callAPI("/api/address-blocklist", {
              method: "POST",
              body: { address, reason: "Removed from graph" },
            });
            await refreshSharedAnnotations();
            if (state.cy) state.viewport = { zoom: state.cy.zoom(), pan: state.cy.pan() };
            renderGraphResponse();
          } catch (err) {
            inspector.textContent = `Remove failed: ${String(err)}`;
          }
        }
        break;
      }
    }
  });

  async function refreshGraphRuns() {
    try {
      const data = await callAPI("/api/actor-tracker/runs");
      state.graphRuns = data.runs || [];
    } catch {
      state.graphRuns = [];
    }
    renderGraphRuns();
  }

  function graphRunOptionLabel(run) {
    const req = run.request || {};
    const actorNames = run.actor_names || "Unknown";
    const start = req.start_time ? new Date(req.start_time).toLocaleDateString() : "?";
    const end = req.end_time ? new Date(req.end_time).toLocaleDateString() : "?";
    const hops = req.max_hops || 0;
    const counts = `${run.node_count}N/${run.edge_count}E`;
    const created = run.created_at ? new Date(run.created_at).toLocaleString() : "";
    return created
      ? `${actorNames} | ${start} - ${end} | ${hops} hops | ${counts} | ${created}`
      : `${actorNames} | ${start} - ${end} | ${hops} hops | ${counts}`;
  }

  function selectedGraphRun() {
    const selectedID = String(state.selectedGraphRunID || "");
    return state.graphRuns.find((run) => String(run.id) === selectedID) || null;
  }

  function renderGraphRuns() {
    if (graphRunCount) graphRunCount.textContent = String(state.graphRuns.length);
    if (!graphRunList) return;
    if (!state.graphRuns.length) {
      state.selectedGraphRunID = null;
      graphRunList.innerHTML = `<div class="empty-state">No past runs yet.</div>`;
      return;
    }

    const selectedExists = state.graphRuns.some((run) => String(run.id) === String(state.selectedGraphRunID || ""));
    state.selectedGraphRunID = selectedExists ? String(state.selectedGraphRunID) : String(state.graphRuns[0].id);

    const options = state.graphRuns
      .map((run) => {
        const runID = String(run.id);
        const selectedAttr = runID === state.selectedGraphRunID ? " selected" : "";
        return `<option value="${escapeHTML(runID)}"${selectedAttr}>${escapeHTML(graphRunOptionLabel(run))}</option>`;
      })
      .join("");

    graphRunList.innerHTML = `
      <article class="actor-card graph-run-dropdown">
        <select id="graph-run-select" aria-label="Select a past run">
          ${options}
        </select>
        <div class="actor-card-actions">
          <button class="secondary" data-load-selected-run type="button">Load</button>
          <button class="secondary" data-delete-selected-run type="button">Delete</button>
        </div>
      </article>
    `;
  }

  async function loadGraphRun(run) {
    const request = normalizeGraphRequest(run.request);
    graphSummary.innerHTML = `<span class="meta-chip">Rebuilding graph…</span>`;
    graphWarnings.innerHTML = "";
    graphStats.innerHTML = "";
    inspector.textContent = "Graph is loading…";
    actionsBody.innerHTML = "";

    try {
      const response = await callAPI("/api/actor-tracker/graph", {
        method: "POST",
        body: request,
      });
      state.actorGraphRequest = request;
      state.baseActorGraph = response;
      state.actorGraph = response;
      state.expandedActors = new Set();
      state.expandedExternalChains = new Set();
      state.expandedHopAddressMap = new Map();
      state.expansionInFlight = false;
      state.viewport = null;
      state.actorGraphFilters.isOpen = false;
      syncGraphFilterStateWithResponse(state.actorGraphFilters, response, { reset: true });
      renderGraphFilterPopover();
      renderGraphResponse();
    } catch (err) {
      graphWarnings.innerHTML = `<span class="warning-chip">${escapeHTML(String(err))}</span>`;
      graphSummary.innerHTML = "";
      graphStats.innerHTML = "";
      inspector.textContent = String(err);
      clearGraphSurface();
    }
  }

  if (graphRunList) {
    graphRunList.addEventListener("change", (e) => {
      const target = e.target instanceof HTMLSelectElement ? e.target : null;
      if (!target || target.id !== "graph-run-select") {
        return;
      }
      state.selectedGraphRunID = target.value;
    });

    graphRunList.addEventListener("click", async (e) => {
      const target = e.target instanceof Element ? e.target : null;
      if (!target) return;

      if (target.closest("[data-load-selected-run]")) {
        const selected = graphRunList.querySelector("#graph-run-select");
        if (selected instanceof HTMLSelectElement) {
          state.selectedGraphRunID = selected.value;
        }
        const run = selectedGraphRun();
        if (!run) return;
        await loadGraphRun(run);
        return;
      }

      if (target.closest("[data-delete-selected-run]")) {
        const selected = graphRunList.querySelector("#graph-run-select");
        if (selected instanceof HTMLSelectElement) {
          state.selectedGraphRunID = selected.value;
        }
        const run = selectedGraphRun();
        if (!run) return;
        try {
          await callAPI(`/api/actor-tracker/runs/${run.id}`, { method: "DELETE" });
          await refreshGraphRuns();
        } catch (err) {
          frontendLog("error", "graph_run_delete_failed", { id: run.id, error: String(err) });
        }
      }
    });
  }

  renderGraphFilterPopover();
  refreshSharedAnnotations();
  refreshActors().catch((err) => {
    actorStatus.textContent = String(err);
  });
  refreshGraphRuns();
}

function decorateVisibleNode(node, actorByIDMap, labelAnnotations) {
  const colors = node.actorIds
    .map((id) => actorByIDMap.get(id)?.color)
    .filter(Boolean);
  const baseColor = node.color || colors[0] || defaultNodeColor(node.kind);
  const pies = buildSharedPie(colors);
  const liveHoldingsAvailable = Boolean(node?.metrics?.live_holdings_available);
  const liveHoldingsUSD = Number(node?.metrics?.live_holdings_usd_spot || 0);
  const liveHoldingsStatus = String(node?.metrics?.live_holdings_status || "").trim().toLowerCase();
  const nodeTotalBondRaw = String(node?.metrics?.node_total_bond || "").trim();
  const liveHoldingsLabel =
    node.kind === "node" && nodeTotalBondRaw
      ? `${formatTokenAmountRaw(nodeTotalBondRaw)} RUNE`
      : liveHoldingsAvailable
      ? formatCompactUSD(liveHoldingsUSD)
      : liveHoldingsStatus === "error"
      ? "Unavailable"
      : "";
  const addr = String((node.metrics && node.metrics.address) || "").trim().toLowerCase();
  const customLabel = labelAnnotations && addr ? labelAnnotations.get(addr) : null;
  const displayLabel = customLabel || node.label || "";
  const showChainLogo = !["pool", "external_cluster"].includes(node.kind);
  const chainLogo = showChainLogo ? (CHAIN_LOGO_URLS[node.chain] || "none") : "none";
  return {
    ...node,
    label: displayLabel,
    displayLabel,
    liveHoldingsLabel,
    liveHoldingsStatus,
    color: baseColor,
    chainLogo,
    borderColor: node.kind === "actor" || node.kind === "actor_address" ? baseColor : node.shared ? "#f4e7a3" : node.kind === "external_cluster" ? "#75d2ba" : "#a2c4ff",
    inspect: {
      id: node.id,
      label: node.label,
      kind: node.kind,
      chain: node.chain,
      stage: node.stage,
      depth: node.depth,
      actor_ids: node.actorIds,
      raw_node_ids: node.rawNodeIDs,
      metrics: node.metrics,
    },
    ...pies,
  };
}

function metaChip(label) {
  return `<span class="meta-chip">${escapeHTML(label)}</span>`;
}

function formatUSD(value) {
  if (!value) {
    return "0";
  }
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(value);
}

function formatCompactUSD(value) {
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

function formatDateTime(value) {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  return date.toLocaleString();
}

function toLocalInputValue(date) {
  const pad = (value) => String(value).padStart(2, "0");
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const minutes = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

function edgeWidth(usdSpot) {
  if (!usdSpot || usdSpot <= 0) {
    return 2.2;
  }
  return Math.min(12, 2 + Math.log10(usdSpot + 1) * 1.6);
}

function edgeColor(actionClass) {
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

function toBigIntOrNull(raw) {
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

function addRawAmountStrings(a, b) {
  const left = toBigIntOrNull(a);
  const right = toBigIntOrNull(b);
  if (left !== null && right !== null) {
    return (left + right).toString();
  }
  const leftNum = Number.parseFloat(String(a ?? "0")) || 0;
  const rightNum = Number.parseFloat(String(b ?? "0")) || 0;
  return String(leftNum + rightNum);
}

function formatTokenAmountRaw(amountRaw) {
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

function cleanAssetName(asset, tokenSymbol) {
  if (tokenSymbol) return tokenSymbol;
  const dotIdx = asset.indexOf(".");
  if (dotIdx < 0) return asset;
  const afterDot = asset.substring(dotIdx + 1);
  const dashIdx = afterDot.indexOf("-");
  return dashIdx < 0 ? afterDot : afterDot.substring(0, dashIdx);
}

function formatAssetDisplay(asset, tokenSymbol) {
  const symbol = cleanAssetName(asset, tokenSymbol);
  const chain = asset.split(".")[0] || "";
  return chain ? `${chain}.${symbol}` : symbol;
}

function formatEdgeTokenSummary(assets) {
  if (!assets || !assets.length) {
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

function formatSwapTokenSummary(assets) {
  if (!assets || !assets.length) {
    return "amount unavailable";
  }
  const inputs = assets.filter((asset) => String(asset.direction || "").toLowerCase() === "in");
  const outputs = assets.filter((asset) => String(asset.direction || "").toLowerCase() === "out");
  if (!inputs.length || !outputs.length) {
    return formatEdgeTokenSummary(assets);
  }

  const input = inputs[0];
  const output = outputs[0];
  const inDisplay = formatAssetDisplay(input.asset, input.token_symbol);
  const outDisplay = formatAssetDisplay(output.asset, output.token_symbol);
  const inputText = `${formatTokenAmountRaw(input.amount_raw)} ${inDisplay}`;
  const outputText = `${formatTokenAmountRaw(output.amount_raw)} ${outDisplay}`;
  const extras = [];
  if (inputs.length > 1) {
    extras.push(`+${inputs.length - 1} in`);
  }
  if (outputs.length > 1) {
    extras.push(`+${outputs.length - 1} out`);
  }
  if (extras.length === 0) {
    return `${inputText} \u2192 ${outputText}`;
  }
  return `${inputText} \u2192 ${outputText} (${extras.join(", ")})`;
}

function defaultNodeColor(kind) {
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

function buildSharedPie(colors) {
  const out = {};
  const unique = uniqueStrings(colors).slice(0, 4);
  if (!unique.length) {
    return out;
  }
  const size = Math.floor(100 / unique.length);
  unique.forEach((color, index) => {
    out[`pie${index + 1}Color`] = color;
    out[`pie${index + 1}Size`] = size;
  });
  return out;
}

function uniqueStrings(values) {
  return Array.from(new Set(values.filter(Boolean)));
}

function uniqueNumbers(values) {
  return Array.from(new Set(values.filter((value) => Number.isFinite(value))));
}

function addressKey(value) {
  return String(value || "").trim();
}

function buildFrontierSeed(address, chain) {
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

function explorerURLForAddress(address, chain) {
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

function shortHash(hash) {
  if (!hash || hash.length < 14) {
    return hash || "";
  }
  return `${hash.slice(0, 10)}…${hash.slice(-8)}`;
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function safeInit(name, initFn, fallback = undefined) {
  try {
    frontendLog("info", "bind_init_started", { name });
    const result = initFn();
    frontendLog("info", "bind_init_completed", { name });
    return result;
  } catch (err) {
    frontendLog("error", "bind_init_failed", { name, error: serializeError(err) });
    return fallback;
  }
}

// ---------------------------------------------------------------------------
// Address Explorer
// ---------------------------------------------------------------------------
function bindAddressExplorer(activateTab) {
  const form = document.getElementById("explorer-form");
  const addressInput = document.getElementById("explorer-address");
  const minUSD = document.getElementById("explorer-min-usd");
  const formStatus = document.getElementById("explorer-form-status");
  const graphSummary = document.getElementById("explorer-query-summary");
  const graphWarnings = document.getElementById("explorer-warnings");
  const graphStats = document.getElementById("explorer-graph-stats");
  const inspector = document.getElementById("explorer-inspector");
  const actionsBody = document.getElementById("explorer-actions-body");
  const graphContainer = document.getElementById("explorer-graph");
  const contextMenu = document.getElementById("graph-context-menu");
  const graphToolbar = document.getElementById("explorer-graph-toolbar");
  const graphFilterPopover = document.getElementById("explorer-graph-filter-popover");
  const graphCard = graphContainer ? graphContainer.closest(".graph-card") : null;
  const directionChooser = document.getElementById("explorer-direction-chooser");
  const directionMessage = directionChooser ? directionChooser.querySelector("p") : null;
  const paginationBar = document.getElementById("explorer-pagination");
  const loadedCountEl = document.getElementById("explorer-loaded-count");
  const loadMoreBtn = document.getElementById("explorer-load-more");
  const explorerRunList = document.getElementById("explorer-run-list");
  const explorerRunCount = document.getElementById("explorer-run-count");

  if (
    !ensureElements("bindAddressExplorer", {
      form,
      addressInput,
      formStatus,
      graphSummary,
      graphWarnings,
      graphStats,
      inspector,
      actionsBody,
      graphContainer,
      contextMenu,
      graphFilterPopover,
      directionChooser,
      directionMessage,
      paginationBar,
      loadedCountEl,
      loadMoreBtn,
      explorerRunList,
      explorerRunCount,
    })
  ) {
    return;
  }

  // Explorer-specific state keys live on the shared `state` object.
  state.explorerGraph = null;
  state.explorerBaseGraph = null;
  state.explorerCy = null;
  state.explorerDirection = null;
  state.explorerHasMore = false;
  state.explorerNextOffset = 0;
  state.explorerLoadedActions = 0;
  state.explorerExpandedHopAddressMap = new Map();
  state.explorerExpansionInFlight = false;
  state.explorerViewport = null;
  state.explorerPreview = null;
  state.explorerRuns = [];
  state.selectedExplorerRunID = null;
  state.explorerGraphRequest = null;
  state.explorerRawAddress = "";
  state.explorerGraphFilters = createGraphFilterState();

  let explorerLabelLayer = null;
  let explorerLabelFrame = 0;
  let explorerNodeTapTimer = null;
  let explorerLastTappedNodeID = "";
  let explorerLastTappedAt = 0;
  let explorerLastPaneContextMenuOpenedAt = 0;
  const nodeDoubleTapWindowMS = 320;
  let explorerContextMenuTarget = null;
  let explorerContextMenuMode = "node";
  let pendingAddress = "";

  function setExplorerFormStatus(message) {
    formStatus.textContent = String(message || "");
  }

  function clearExplorerTables() {
    actionsBody.innerHTML = "";
  }

  function showExplorerPlaceholder(message) {
    clearExplorerGraphSurface();
    graphContainer.innerHTML = `<div class="empty-state">${escapeHTML(String(message || "No graph loaded."))}</div>`;
  }

  function explorerCurrentMeta() {
    return state.explorerGraph || state.explorerPreview || null;
  }

  function explorerSeedChips(meta) {
    const activeChains = Array.isArray(meta?.active_chains) ? meta.active_chains : [];
    return activeChains.map((chain) => metaChip(chain));
  }

  function renderExplorerFilterPopover() {
    graphFilterPopover.innerHTML = graphFilterPopoverMarkup(state.explorerGraphFilters);
    graphFilterPopover.hidden = !state.explorerGraphFilters.isOpen;
    updateGraphFilterButtonState(graphToolbar, state.explorerGraphFilters);
  }

  function hideExplorerFilterPopover() {
    if (!state.explorerGraphFilters.isOpen) {
      updateGraphFilterButtonState(graphToolbar, state.explorerGraphFilters);
      return;
    }
    state.explorerGraphFilters.isOpen = false;
    renderExplorerFilterPopover();
  }

  function toggleExplorerFilterPopover() {
    if (!state.explorerGraph) {
      return;
    }
    state.explorerGraphFilters.isOpen = !state.explorerGraphFilters.isOpen;
    renderExplorerFilterPopover();
  }

  graphFilterPopover.addEventListener("change", (e) => {
    const target = e.target instanceof HTMLInputElement ? e.target : null;
    if (!target) {
      return;
    }
    if (target.dataset.filterTxn) {
      state.explorerGraphFilters.txnTypes[target.dataset.filterTxn] = target.checked;
      renderExplorerFilterPopover();
      renderExplorerGraphResponse();
      return;
    }
    if (target.dataset.filterChain) {
      const chain = String(target.dataset.filterChain || "");
      const selected = new Set(state.explorerGraphFilters.selectedChains || []);
      if (target.checked) {
        selected.add(chain);
      } else {
        selected.delete(chain);
      }
      state.explorerGraphFilters.selectedChains = Array.from(selected).sort();
      renderExplorerFilterPopover();
      renderExplorerGraphResponse();
      return;
    }
    if (target.dataset.filterTime === "start") {
      setGraphFilterDateValue(state.explorerGraphFilters, "startTime", target.value);
      renderExplorerFilterPopover();
      renderExplorerGraphResponse();
      return;
    }
    if (target.dataset.filterTime === "end") {
      setGraphFilterDateValue(state.explorerGraphFilters, "endTime", target.value);
      renderExplorerFilterPopover();
      renderExplorerGraphResponse();
      return;
    }
    if (target.dataset.filterUsd === "min") {
      setGraphFilterNumberValue(state.explorerGraphFilters, "minTxnUSD", target.value);
      renderExplorerFilterPopover();
      renderExplorerGraphResponse();
      return;
    }
    if (target.dataset.filterUsd === "max") {
      setGraphFilterNumberValue(state.explorerGraphFilters, "maxTxnUSD", target.value);
      renderExplorerFilterPopover();
      renderExplorerGraphResponse();
    }
  });

  graphFilterPopover.addEventListener("click", (e) => {
    const target = e.target instanceof Element ? e.target.closest("[data-filter-reset]") : null;
    if (!target) {
      return;
    }
    resetGraphFilters(state.explorerGraphFilters);
    renderExplorerFilterPopover();
    renderExplorerGraphResponse();
  });

  graphContainer.addEventListener("click", (e) => {
    const target = e.target instanceof Element ? e.target.closest("[data-graph-reset-filters='explorer']") : null;
    if (!target) {
      return;
    }
    resetGraphFilters(state.explorerGraphFilters);
    renderExplorerFilterPopover();
    renderExplorerGraphResponse();
  });

  document.addEventListener("mousedown", (e) => {
    if (!state.explorerGraphFilters.isOpen) {
      return;
    }
    const target = e.target instanceof Node ? e.target : null;
    const filterButton = graphToolbar?.querySelector('[data-graph-action="filters"]');
    if ((target && graphFilterPopover.contains(target)) || (filterButton && target && filterButton.contains(target))) {
      return;
    }
    hideExplorerFilterPopover();
  });

  graphContainer.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    if (explorerGraphNodeAtClientPoint(e.clientX, e.clientY)) {
      return;
    }
    showExplorerPaneContextMenu({ x: e.clientX, y: e.clientY });
  });
  attachGraphPointerControls({
    container: graphContainer,
    getCy: () => state.explorerCy,
    hitNodeAtClientPoint: (clientX, clientY) => explorerGraphNodeAtClientPoint(clientX, clientY),
  });

  function explorerGraphNodeAtClientPoint(clientX, clientY) {
    if (!state.explorerCy) return null;
    const rect = graphContainer.getBoundingClientRect();
    const rx = clientX - rect.left;
    const ry = clientY - rect.top;
    if (rx < 0 || ry < 0 || rx > rect.width || ry > rect.height) return null;
    const nodes = state.explorerCy.nodes();
    for (let i = nodes.length - 1; i >= 0; i--) {
      const node = nodes[i];
      if (typeof node.visible === "function" && !node.visible()) continue;
      const box = typeof node.renderedBoundingBox === "function" ? node.renderedBoundingBox({ includeLabels: false, includeOverlays: false }) : null;
      if (box && rx >= box.x1 && rx <= box.x2 && ry >= box.y1 && ry <= box.y2) return node;
    }
    return null;
  }

  async function refreshExplorerRuns() {
    try {
      const data = await callAPI("/api/address-explorer/runs");
      state.explorerRuns = data.runs || [];
    } catch {
      state.explorerRuns = [];
    }
    renderExplorerRuns();
  }

  function explorerRunOptionLabel(run) {
    const summary = String(run.summary || "Unknown run");
    const counts = `${run.node_count}N/${run.edge_count}E`;
    const created = run.created_at ? new Date(run.created_at).toLocaleString() : "";
    return created ? `${summary} | ${counts} | ${created}` : `${summary} | ${counts}`;
  }

  function selectedExplorerRun() {
    const selectedID = String(state.selectedExplorerRunID || "");
    return state.explorerRuns.find((run) => String(run.id) === selectedID) || null;
  }

  function renderExplorerRuns() {
    explorerRunCount.textContent = String(state.explorerRuns.length);
    if (!state.explorerRuns.length) {
      state.selectedExplorerRunID = null;
      explorerRunList.innerHTML = `<div class="empty-state">No past runs yet.</div>`;
      return;
    }

    const selectedExists = state.explorerRuns.some((run) => String(run.id) === String(state.selectedExplorerRunID || ""));
    state.selectedExplorerRunID = selectedExists ? String(state.selectedExplorerRunID) : String(state.explorerRuns[0].id);

    const options = state.explorerRuns
      .map((run) => {
        const runID = String(run.id);
        const selectedAttr = runID === state.selectedExplorerRunID ? " selected" : "";
        return `<option value="${escapeHTML(runID)}"${selectedAttr}>${escapeHTML(explorerRunOptionLabel(run))}</option>`;
      })
      .join("");

    explorerRunList.innerHTML = `
      <article class="actor-card graph-run-dropdown">
        <select id="explorer-run-select" aria-label="Select a past explorer run">
          ${options}
        </select>
        <div class="actor-card-actions">
          <button class="secondary" data-load-selected-explorer-run type="button">Load</button>
          <button class="secondary" data-delete-selected-explorer-run type="button">Delete</button>
        </div>
      </article>
    `;
  }

  function renderExplorerMeta(visible, filteredActions) {
    const meta = explorerCurrentMeta();
    if (!meta) {
      graphSummary.innerHTML = "";
      graphWarnings.innerHTML = "";
      graphStats.innerHTML = "";
      return;
    }
    const q = meta.query || {};
    const chips = [metaChip(q.address ? shortHash(q.address) : "")].concat(explorerSeedChips(meta));
    if (q.direction) {
      chips.push(metaChip(`${q.direction} direction`));
    }
    if (meta.direction_required) {
      chips.push(metaChip("Choose direction"));
    }
    if (state.explorerExpandedHopAddressMap.size > 0) {
      chips.push(metaChip(`+${state.explorerExpandedHopAddressMap.size} expanded edges`));
    }
    if (state.explorerGraph && graphFiltersAreActive(state.explorerGraphFilters)) {
      chips.push(metaChip("Filters active"));
    }
    graphSummary.innerHTML = chips.join("");

    const warnings = Array.isArray(meta.warnings) ? meta.warnings : [];
    graphWarnings.innerHTML = warnings.length
      ? warnings.map((w) => `<span class="warning-chip">${escapeHTML(w)}</span>`).join("")
      : "";

    if (state.explorerGraph) {
      const filteredNodeCount = Array.isArray(visible?.nodes) ? visible.nodes.length : 0;
      const filteredEdgeCount = Array.isArray(visible?.edges) ? visible.edges.length : 0;
      const filteredActionCount = Array.isArray(filteredActions) ? filteredActions.length : 0;
      const totalNodeCount = Number((state.explorerGraph.stats || {}).node_count || 0);
      const totalEdgeCount = Number((state.explorerGraph.stats || {}).edge_count || 0);
      const totalActionCount = Number((state.explorerGraph.stats || {}).supporting_action_count || 0);
      const showNodeFraction = graphFiltersAreActive(state.explorerGraphFilters) || filteredNodeCount !== totalNodeCount;
      const showEdgeFraction = graphFiltersAreActive(state.explorerGraphFilters) || filteredEdgeCount !== totalEdgeCount;
      const showActionFraction = graphFiltersAreActive(state.explorerGraphFilters) || filteredActionCount !== totalActionCount;
      graphStats.innerHTML = [
        metaChip(showNodeFraction ? `${filteredNodeCount} / ${totalNodeCount} nodes` : `${totalNodeCount} nodes`),
        metaChip(showEdgeFraction ? `${filteredEdgeCount} / ${totalEdgeCount} edges` : `${totalEdgeCount} edges`),
        metaChip(showActionFraction ? `${filteredActionCount} / ${totalActionCount} actions` : `${totalActionCount} actions`),
      ].join("");
      return;
    }

    graphStats.innerHTML = [
      metaChip(`${(meta.active_chains || []).length} active chains`),
      meta.has_more ? metaChip("500+ actions detected") : metaChip("Ready to load"),
    ].join("");
  }

  async function loadExplorerGraph(rawRequest, options = {}) {
    const request = normalizeExplorerRequest({
      ...rawRequest,
      mode: "graph",
      direction: rawRequest?.direction || "newest",
    });
    const merge = Boolean(options.merge);
    const preserveViewport = Boolean(options.preserveViewport);
    const refreshRuns = Boolean(options.refreshRuns);

    if (!request.address) {
      inspector.textContent = "Enter a wallet address to explore.";
      return;
    }

    state.explorerRawAddress = request.address;
    pendingAddress = request.address;
    state.explorerDirection = request.direction || "newest";
    setExplorerFormStatus(merge ? "Loading next batch..." : "Loading graph...");
    if (!merge) {
      clearExplorerTables();
      clearExplorerGraphSurface();
      inspector.textContent = "Graph is loading…";
    }

    if (merge && preserveViewport && state.explorerCy) {
      state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
    }

    const resp = await callAPI("/api/address-explorer/graph", {
      method: "POST",
      body: request,
    });

    state.explorerPreview = resp;
    state.explorerHasMore = resp.has_more;
    state.explorerNextOffset = resp.next_offset;
    state.explorerGraphRequest = { ...request, offset: 0 };

    if (merge && state.explorerGraph) {
      state.explorerGraph = mergeExplorerGraphResponse(state.explorerGraph, resp);
      state.explorerBaseGraph = mergeExplorerGraphResponse(state.explorerBaseGraph, resp);
      state.explorerLoadedActions += resp.loaded_actions || 0;
    } else {
      state.explorerGraph = resp;
      state.explorerBaseGraph = resp;
      state.explorerLoadedActions = resp.loaded_actions || 0;
      state.explorerExpandedHopAddressMap = new Map();
      state.explorerGraphFilters.isOpen = false;
      syncGraphFilterStateWithResponse(state.explorerGraphFilters, resp, { reset: true });
    }

    renderExplorerFilterPopover();
    renderExplorerGraphResponse();
    updatePaginationBar();
    setExplorerFormStatus("");
    inspector.textContent = merge
      ? `Loaded ${state.explorerLoadedActions} total actions.`
      : "Select a node or edge in the graph.";
    if (refreshRuns) {
      await refreshExplorerRuns();
    }
  }

  async function requestExplorerPreview() {
    const address = addressInput.value.trim();
    if (!address) {
      inspector.textContent = "Enter a wallet address to explore.";
      return;
    }

    pendingAddress = address;
    state.explorerRawAddress = address;
    state.explorerGraph = null;
    state.explorerBaseGraph = null;
    state.explorerPreview = null;
    state.explorerDirection = null;
    state.explorerHasMore = false;
    state.explorerNextOffset = 0;
    state.explorerLoadedActions = 0;
    state.explorerExpandedHopAddressMap = new Map();
    state.explorerViewport = null;
    state.explorerGraphFilters.isOpen = false;
    directionChooser.style.display = "none";
    paginationBar.style.display = "none";
    clearExplorerTables();
    clearExplorerGraphSurface();
    graphSummary.innerHTML = "";
    graphWarnings.innerHTML = "";
    graphStats.innerHTML = "";
    renderExplorerFilterPopover();
    inspector.textContent = "Checking address activity…";
    setExplorerFormStatus("Checking address activity…");

    const previewRequest = normalizeExplorerRequest({
      address,
      min_usd: Number(minUSD.value) || 0,
      mode: "preview",
      direction: "",
      offset: 0,
      batch_size: 10,
    });

    const resp = await callAPI("/api/address-explorer/graph", {
      method: "POST",
      body: previewRequest,
    });

    state.explorerPreview = resp;
    renderExplorerMeta();

    if (resp.direction_required) {
      state.explorerDirection = null;
      directionMessage.textContent = `This address spans ${Math.max((resp.active_chains || []).length, 1)} active chain(s) and more than 500 actions. Choose loading direction:`;
      directionChooser.style.display = "block";
      setExplorerFormStatus("Choose a loading direction.");
      showExplorerPlaceholder("Choose newest or oldest to load the first graph batch.");
      inspector.textContent = "Choose newest or oldest to load the first graph batch.";
      return;
    }

    directionChooser.style.display = "none";
    setExplorerFormStatus("Loading graph…");
    await loadExplorerGraph({
      address,
      min_usd: Number(minUSD.value) || 0,
      direction: "newest",
      offset: 0,
      batch_size: 10,
    }, { refreshRuns: true });
  }

  function updatePaginationBar() {
    if (state.explorerGraph && state.explorerDirection && (state.explorerHasMore || state.explorerLoadedActions > 0)) {
      paginationBar.style.display = "flex";
      loadedCountEl.textContent = `${state.explorerLoadedActions} actions loaded`;
      loadMoreBtn.style.display = state.explorerHasMore ? "" : "none";
    } else {
      paginationBar.style.display = "none";
    }
  }

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    try {
      await requestExplorerPreview();
    } catch (err) {
      setExplorerFormStatus("");
      inspector.textContent = `Explorer failed: ${String(err)}`;
      graphWarnings.innerHTML = `<span class="warning-chip">${escapeHTML(String(err))}</span>`;
    }
  });

  directionChooser.addEventListener("click", async (e) => {
    const btn = e.target instanceof Element ? e.target.closest("button[data-direction]") : null;
    if (!btn) return;
    const direction = btn.dataset.direction;
    if (direction !== "newest" && direction !== "oldest") return;
    directionChooser.style.display = "none";
    try {
      await loadExplorerGraph({
        address: pendingAddress,
        min_usd: Number(minUSD.value) || 0,
        direction,
        offset: 0,
        batch_size: 10,
      }, { refreshRuns: true });
    } catch (err) {
      setExplorerFormStatus("");
      inspector.textContent = `Explorer failed: ${String(err)}`;
    }
  });

  loadMoreBtn.addEventListener("click", async () => {
    if (!state.explorerHasMore || !state.explorerDirection || !state.explorerGraphRequest) return;
    loadMoreBtn.disabled = true;
    loadMoreBtn.textContent = "Loading…";
    inspector.textContent = "Loading next batch…";
    try {
      await loadExplorerGraph({
        ...state.explorerGraphRequest,
        address: pendingAddress || state.explorerGraphRequest.address,
        direction: state.explorerDirection,
        offset: state.explorerNextOffset,
      }, { merge: true, preserveViewport: true, refreshRuns: false });
    } catch (err) {
      setExplorerFormStatus("");
      inspector.textContent = `Load more failed: ${String(err)}`;
    } finally {
      loadMoreBtn.disabled = false;
      loadMoreBtn.textContent = "Load next 500";
    }
  });

  explorerRunList.addEventListener("change", (e) => {
    const target = e.target instanceof HTMLSelectElement ? e.target : null;
    if (!target || target.id !== "explorer-run-select") return;
    state.selectedExplorerRunID = target.value;
  });

  explorerRunList.addEventListener("click", async (e) => {
    const target = e.target instanceof Element ? e.target : null;
    if (!target) return;

    if (target.closest("[data-load-selected-explorer-run]")) {
      const selected = explorerRunList.querySelector("#explorer-run-select");
      if (selected instanceof HTMLSelectElement) {
        state.selectedExplorerRunID = selected.value;
      }
      const run = selectedExplorerRun();
      if (!run) return;
      const request = normalizeExplorerRequest(run.request);
      addressInput.value = request.address || "";
      minUSD.value = Number.isFinite(Number(request.min_usd)) ? String(request.min_usd) : "0";
      directionChooser.style.display = "none";
      try {
        await loadExplorerGraph({
          ...request,
          mode: "graph",
          offset: 0,
          batch_size: request.batch_size || 10,
        }, { refreshRuns: false });
      } catch (err) {
        setExplorerFormStatus("");
        inspector.textContent = `Explorer failed: ${String(err)}`;
      }
      return;
    }

    if (target.closest("[data-delete-selected-explorer-run]")) {
      const selected = explorerRunList.querySelector("#explorer-run-select");
      if (selected instanceof HTMLSelectElement) {
        state.selectedExplorerRunID = selected.value;
      }
      const run = selectedExplorerRun();
      if (!run) return;
      try {
        await callAPI(`/api/address-explorer/runs/${run.id}`, { method: "DELETE" });
        await refreshExplorerRuns();
      } catch (err) {
        frontendLog("error", "explorer_run_delete_failed", { id: run.id, error: String(err) });
      }
    }
  });

  // --- Graph merge ---
  function mergeExplorerGraphResponse(base, expansion) {
    if (!base) return expansion;

    function nodeMergeKey(node) {
      if (!node) return "";
      const metrics = node.metrics || {};
      const address = String(metrics.address || "").trim().toLowerCase();
      const pool = String(metrics.pool || "").trim().toUpperCase();
      const chain = String(node.chain || "").trim().toUpperCase();
      const kind = String(node.kind || "").trim().toLowerCase();
      if (address) return `${kind}|${chain}|${address}`;
      if (pool) return `${kind}|${pool}`;
      return String(node.id || "");
    }

    function edgeMergeKey(edge, nodeAlias) {
      const from = nodeAlias.get(edge.from) || edge.from;
      const to = nodeAlias.get(edge.to) || edge.to;
      let key = `${from}|${to}|${edge.action_key || edge.action_class}`;
      if (edge.validator_address && String(edge.action_key || edge.action_class || "").toLowerCase().includes("rebond")) {
        key += `|validator:${edge.validator_address}`;
      }
      return key;
    }

    const nodeMap = new Map();
    const nodeAlias = new Map();
    const nodeKeyToID = new Map();

    function mergeNode(node) {
      const mergeKey = nodeMergeKey(node);
      const existingID = nodeKeyToID.get(mergeKey) || node.id;
      nodeAlias.set(node.id, existingID);
      const existing = nodeMap.get(existingID);
      if (!existing) {
        nodeKeyToID.set(mergeKey, existingID);
        nodeMap.set(existingID, { ...node, id: existingID, metrics: { ...(node.metrics || {}) } });
        return;
      }
      existing.depth = Math.min(Number(existing.depth || 0), Number(node.depth || 0));
      existing.metrics = { ...(existing.metrics || {}), ...(node.metrics || {}) };
    }

    (base.nodes || []).forEach(mergeNode);
    (expansion.nodes || []).forEach(mergeNode);

    function cloneMergedEdge(edge, canonicalID) {
      const transactions = mergeEdgeTransactions([], normalizeEdgeTransactions(edge));
      const summary = summarizeTransactions(transactions);
      return {
        ...edge,
        id: canonicalID,
        from: nodeAlias.get(edge.from) || edge.from,
        to: nodeAlias.get(edge.to) || edge.to,
        transactions,
        tx_ids: summary.tx_ids,
        heights: summary.heights,
        assets: summary.assets,
        usd_spot: summary.usd_spot,
      };
    }

    const edgeMap = new Map();
    function addEdge(edge) {
      const canonicalID = edgeMergeKey(edge, nodeAlias);
      const existing = edgeMap.get(canonicalID);
      if (!existing) {
        edgeMap.set(canonicalID, cloneMergedEdge(edge, canonicalID));
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
    (base.edges || []).forEach(addEdge);
    (expansion.edges || []).forEach(addEdge);

    const actionKey = (a) => `${a.tx_id}|${a.action_key || a.action_class}|${a.from_node}|${a.to_node}`;
    const actionMap = new Map((base.supporting_actions || []).map((a) => [actionKey(a), a]));
    (expansion.supporting_actions || []).forEach((a) => { if (!actionMap.has(actionKey(a))) actionMap.set(actionKey(a), a); });

    return {
      ...base,
      mode: expansion.mode || base.mode,
      raw_address: expansion.raw_address || base.raw_address,
      address: expansion.address || base.address,
      query: { ...(base.query || {}), ...(expansion.query || {}) },
      active_chains: Array.isArray(expansion.active_chains) && expansion.active_chains.length ? expansion.active_chains : base.active_chains || [],
      seed_summaries: Array.isArray(expansion.seed_summaries) && expansion.seed_summaries.length ? expansion.seed_summaries : base.seed_summaries || [],
      direction_required: Boolean(expansion.direction_required),
      run_label: expansion.run_label || base.run_label || "",
      warnings: uniqueStrings((base.warnings || []).concat(expansion.warnings || [])),
      nodes: Array.from(nodeMap.values()),
      edges: Array.from(edgeMap.values()),
      supporting_actions: Array.from(actionMap.values()),
      loaded_actions: Number(base.loaded_actions || 0) + Number(expansion.loaded_actions || 0),
      stats: {
        ...(base.stats || {}),
        ...(expansion.stats || {}),
        node_count: nodeMap.size,
        edge_count: edgeMap.size,
        supporting_action_count: actionMap.size,
      },
    };
  }

  // --- Render pipeline ---
  function clearExplorerGraphSurface() {
    if (explorerLabelFrame) {
      window.cancelAnimationFrame(explorerLabelFrame);
      explorerLabelFrame = 0;
    }
    explorerLabelLayer = null;
    graphContainer.innerHTML = "";
    if (state.explorerCy) {
      state.explorerCy.destroy();
      state.explorerCy = null;
    }
    inspector.textContent = "Select a node or edge in the graph.";
  }

  function renderExplorerGraphResponse() {
    if (!state.explorerGraph) {
      state.explorerGraphFilters.isOpen = false;
      renderExplorerFilterPopover();
      renderExplorerMeta();
      clearExplorerGraphSurface();
      clearExplorerTables();
      return;
    }
    syncGraphFilterStateWithResponse(state.explorerGraphFilters, state.explorerGraph);
    const visible = explorerDeriveVisibleGraph(state.explorerGraph);
    const filteredActions = filterSupportingActions(state.explorerGraph.supporting_actions || [], state.explorerGraph, state.explorerGraphFilters);
    renderExplorerFilterPopover();
    renderExplorerMeta(visible, filteredActions);
    renderExplorerActions(filteredActions);
    renderExplorerGraph(visible);
  }

  function renderExplorerSummary() {
    const q = state.explorerGraph.query || {};
    const chips = [
      metaChip(q.address ? shortHash(q.address) : ""),
      metaChip(`${q.direction || "newest"} direction`),
    ];
    if (state.explorerExpandedHopAddressMap.size > 0) {
      chips.push(metaChip(`+${state.explorerExpandedHopAddressMap.size} expanded edges`));
    }
    graphSummary.innerHTML = chips.join("");
    graphStats.innerHTML = [
      metaChip(`${(state.explorerGraph.stats || {}).node_count || 0} nodes`),
      metaChip(`${(state.explorerGraph.stats || {}).edge_count || 0} edges`),
      metaChip(`${(state.explorerGraph.stats || {}).supporting_action_count || 0} actions`),
    ].join("");
  }

  function renderExplorerWarnings() {
    const warnings = state.explorerGraph.warnings || [];
    graphWarnings.innerHTML = warnings.length
      ? warnings.map((w) => `<span class="warning-chip">${escapeHTML(w)}</span>`).join("")
      : "";
  }

  function renderExplorerActions(actions) {
    if (!actions.length) {
      actionsBody.innerHTML = `<tr><td colspan="6" class="empty-state">No supporting actions.</td></tr>`;
      return;
    }
    actionsBody.innerHTML = actions
      .map(
        (a) => `<tr>
          <td>${escapeHTML(formatDateTime(a.time))}</td>
          <td>${escapeHTML(a.action_label || a.action_class || "")}</td>
          <td><span class="mono">${escapeHTML(shortHash(a.tx_id))}</span></td>
          <td class="mono">${escapeHTML(a.primary_asset || "")}</td>
          <td class="mono">${escapeHTML(a.amount_raw || "")}</td>
          <td>${formatUSD(a.usd_spot)}</td>
        </tr>`
      )
      .join("");
  }

  // --- Derive visible graph (simplified: no actor collapsing, no external cluster collapsing) ---
  function explorerDeriveVisibleGraph(response) {
    const rawNodes = response.nodes || [];
    const rawEdges = response.edges || [];
    const rawNodeByID = new Map(rawNodes.map((n) => [n.id, n]));
    const filterState = state.explorerGraphFilters || createGraphFilterState();
    const filteredRawEdges = [];

    const blocklistedAddresses = new Set(state.blocklist.map((b) => String(b.normalized_address || "").toLowerCase()));
    const asgardAddresses = new Set(
      state.annotations.filter((a) => a.kind === "asgard_vault").map((a) => String(a.normalized_address || "").toLowerCase())
    );
    const labelAnnotations = new Map(
      state.annotations
        .filter((a) => a && a.kind === "label" && a.value !== null && a.value !== undefined)
        .map((a) => [String(a.normalized_address || "").toLowerCase(), String(a.value)])
    );
    const hiddenAddresses = new Set([...blocklistedAddresses, ...asgardAddresses]);

    function isHiddenAddress(node) {
      const addr = String((node.metrics && node.metrics.address) || "").trim().toLowerCase();
      return addr && hiddenAddresses.has(addr);
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

    const visibleNodes = new Map();
    const visibleEdges = new Map();

    function mapNodeID(rawNode) {
      if (!rawNode) return null;
      if (isHiddenAddress(rawNode)) return null;
      return rawNode.id;
    }

    function ensureVisibleNode(rawNode, mappedID) {
      if (!mappedID) return;
      const existing = visibleNodes.get(mappedID) || {
        id: mappedID,
        label: rawNode.label,
        kind: rawNode.kind,
        chain: rawNode.chain,
        stage: rawNode.stage,
        depth: rawNode.depth,
        actorIds: [],
        shared: false,
        collapsed: false,
        rawNodeIDs: [],
        metrics: { ...(rawNode.metrics || {}), live_holdings_usd_spot: 0 },
        color: rawNode.kind === "explorer_target" ? "#e67e22" : defaultNodeColor(rawNode.kind),
      };
      if (!existing.rawNodeIDs.includes(rawNode.id)) {
        existing.rawNodeIDs.push(rawNode.id);
        existing.depth = Math.min(existing.depth, rawNode.depth);
        existing.metrics.live_holdings_usd_spot =
          Number(existing.metrics.live_holdings_usd_spot || 0) + Number(rawNode?.metrics?.live_holdings_usd_spot || 0);
        existing.metrics.live_holdings_available =
          Boolean(existing.metrics.live_holdings_available) || Boolean(rawNode?.metrics?.live_holdings_available);
        existing.metrics.live_holdings_status = mergeLiveHoldingsStatus(
          existing.metrics.live_holdings_status,
          rawNode?.metrics?.live_holdings_status
        );
      }
      visibleNodes.set(mappedID, existing);
    }

    function addVisibleEdge(rawEdge) {
      const sourceNode = rawNodeByID.get(rawEdge.from);
      const targetNode = rawNodeByID.get(rawEdge.to);
      const from = mapNodeID(sourceNode);
      const to = mapNodeID(targetNode);
      if (!from || !to || from === to) return false;
      ensureVisibleNode(sourceNode, from);
      ensureVisibleNode(targetNode, to);

      const edgeID = graphVisibleEdgeKey(rawEdge, from, to);
      const existing = visibleEdges.get(edgeID) || {
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
        actorIds: [],
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
      existing.rawEdgeIDs = uniqueStrings(existing.rawEdgeIDs.concat(rawEdge.id));
      existing.actionClasses = uniqueStrings(existing.actionClasses.concat(String(rawEdge.action_class || "").trim()).filter(Boolean));
      existing.actionKeys = uniqueStrings(existing.actionKeys.concat(String(rawEdge.action_key || rawEdge.action_class || "").trim()).filter(Boolean));
      existing.actionLabels = uniqueStrings(existing.actionLabels.concat(String(rawEdge.action_label || rawEdge.action_class || "").trim()).filter(Boolean));
      existing.actionDomains = uniqueStrings(existing.actionDomains.concat(String(rawEdge.action_domain || rawEdge.action_class || "").trim()).filter(Boolean));
      const txnBucket = graphTxnBucket(rawEdge.action_class, rawEdge.action_key, rawEdge.action_label);
      if (txnBucket) {
        existing.txnBuckets = uniqueStrings(existing.txnBuckets.concat(txnBucket));
      }
      if (rawEdge.validator_address) {
        existing.validatorAddresses = uniqueStrings(existing.validatorAddresses.concat(String(rawEdge.validator_address).trim()));
      }
      if (rawEdge.validator_label) {
        existing.validatorLabels = uniqueStrings(existing.validatorLabels.concat(String(rawEdge.validator_label).trim()));
      }
      if (rawEdge.contract_type) {
        existing.contractTypes = uniqueStrings(existing.contractTypes.concat(String(rawEdge.contract_type).trim()));
      }
      if (rawEdge.contract_protocol) {
        existing.contractProtocols = uniqueStrings(existing.contractProtocols.concat(String(rawEdge.contract_protocol).trim()));
      }
      const metadata = resolveVisibleEdgeMetadata(existing);
      existing.actionClass = metadata.actionClass;
      existing.actionKey = metadata.actionKey;
      existing.actionLabel = metadata.actionLabel;
      existing.actionDomain = metadata.actionDomain;
      existing.validatorAddress = metadata.validatorAddress;
      existing.validatorLabel = metadata.validatorLabel;
      existing.contractType = metadata.contractType;
      existing.contractProtocol = metadata.contractProtocol;
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
      existing.inspect.action_classes = metadata.actionClasses;
      existing.inspect.action_keys = metadata.actionKeys;
      existing.inspect.action_labels = metadata.actionLabels;
      existing.inspect.action_domains = metadata.actionDomains;
      existing.inspect.action_buckets = metadata.txnBuckets;
      existing.inspect.validator_addresses = metadata.validatorAddresses;
      existing.inspect.validator_labels = metadata.validatorLabels;
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
      existing.inspect.edges.push(rawEdge);
      existing.width = edgeWidth(existing.usdSpot);
      visibleEdges.set(edgeID, existing);
      return true;
    }

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

    const emptyActorMap = new Map();
    const nodes = Array.from(visibleNodes.values()).map((n) => decorateVisibleNode(n, emptyActorMap, labelAnnotations));
    const edges = Array.from(visibleEdges.values()).map((edge) => {
      const aggregatedAssets = Object.entries(edge.assetTotals)
        .map(([, b]) => ({ asset: b.asset, direction: b.direction || "", amount_raw: b.amountRaw, usd_spot: b.usdSpot, token_symbol: b.tokenSymbol || "" }))
        .sort((a, b) => Number(b.usd_spot || 0) - Number(a.usd_spot || 0));
      const tokenSummary = edge.actionClass === "swaps" ? formatSwapTokenSummary(aggregatedAssets) : formatEdgeTokenSummary(aggregatedAssets);
      const usdSummary = formatUSD(edge.usdSpot);
      const edgeActionLabel = formatVisibleEdgeActionLabel(edge);
      const edgeLabel = edge.actionClass === "ownership" ? "" : `${edgeActionLabel} · ${tokenSummary}\n${usdSummary}`;
      return {
        ...edge,
        edgeLabel,
        lineColor: edgeColor(edge.actionClass),
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

  function mergeLiveHoldingsStatus(current, incoming) {
    const c = String(current || "").trim().toLowerCase();
    const i = String(incoming || "").trim().toLowerCase();
    if (c === "available" || i === "available") return "available";
    if (c === "error" || i === "error") return "error";
    return c || i;
  }

  // --- Graph stylesheet (adds explorer_target style) ---
  function explorerGraphStylesheet() {
    return [
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
      { selector: "node:selected", style: { "border-width": 4, "border-color": "#ffdd44", "overlay-color": "rgba(255,221,68,0.15)", "overlay-opacity": 0.3 } },
      { selector: "node[kind = 'explorer_target']", style: { shape: "hexagon", width: 110, height: 100, "background-color": "#e67e22", "border-color": "#f5c76e", "border-width": 4, "font-size": 13 } },
      { selector: "node[kind = 'pool']", style: { shape: "diamond", width: 84, height: 84, "background-color": "#1e4f8f" } },
      { selector: "node[kind = 'actor']", style: { "background-color": "#0d1b2a", "background-width": "50%", "background-height": "80%", "background-image-opacity": 1, "background-opacity": 1, "border-width": 4, "pie-size": "0%" } },
      { selector: "node[kind = 'actor_address']", style: { shape: "ellipse", width: 62, height: 62, "background-color": "#0d1b2a", "background-opacity": 1, "background-width": "60%", "background-height": "60%", "background-image-opacity": 1, "border-width": 4, "pie-size": "0%" } },
      { selector: "node[kind = 'external_address']", style: { shape: "ellipse", width: 62, height: 62 } },
      { selector: "node[kind = 'node']", style: { shape: "octagon", width: 94, height: 94, "background-color": "#c86b1f", "border-color": "#ffe0b8", "border-width": 4, color: "#fff7ea", "font-size": 12 } },
      { selector: "node[kind = 'contract_address']", style: { shape: "round-rectangle", width: 108, height: 60, "background-color": "#915a2b" } },
      { selector: "node[kind = 'bond_address']", style: { shape: "tag", "background-color": "#654590" } },
      { selector: "node[kind = 'inbound'], node[kind = 'router']", style: { shape: "round-rectangle", "background-color": "#176666" } },
      { selector: "node[kind = 'external_cluster']", style: { shape: "barrel", width: 120, height: 56, "background-color": "#164a47" } },
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
      { selector: "edge[actionClass = 'ownership']", style: { label: "", width: 1.4, "line-style": "dashed", "line-color": "#7a94bb", "target-arrow-color": "#7a94bb", opacity: 0.5 } },
    ];
  }

  // --- ELK layout ---
  async function explorerApplyElkLayout(cy, nodes) {
    const elk = new ELK();
    const graph = {
      id: "root",
      layoutOptions: {
        "elk.algorithm": "layered",
        "elk.direction": "RIGHT",
        "elk.edgeRouting": "POLYLINE",
        "elk.layered.spacing.nodeNodeBetweenLayers": "110",
        "elk.spacing.nodeNode": "42",
        "elk.padding": "[top=32,left=32,bottom=32,right=32]",
      },
      children: nodes.map((n) => ({
        id: n.id,
        width: n.kind === "explorer_target" ? 120 : n.kind === "pool" ? 96 : 84,
        height: n.kind === "explorer_target" ? 108 : n.kind === "pool" ? 96 : 72,
      })),
      edges: cy.edges().toArray().map((e) => ({
        id: e.id(),
        sources: [e.data("source")],
        targets: [e.data("target")],
      })),
    };
    const result = await elk.layout(graph);
    const positions = new Map((result.children || []).map((c) => [c.id, { x: c.x, y: c.y }]));
    cy.layout({
      name: "preset",
      fit: false,
      animate: false,
      positions(node) { return positions.get(node.id()) || { x: node.data("depth") * 180, y: 80 }; },
    }).run();
  }

  // --- Node label rendering ---
  function ensureExplorerLabelLayer() {
    if (explorerLabelLayer && explorerLabelLayer.isConnected) return explorerLabelLayer;
    explorerLabelLayer = document.createElement("div");
    explorerLabelLayer.className = "graph-label-layer";
    graphContainer.appendChild(explorerLabelLayer);
    return explorerLabelLayer;
  }

  function renderExplorerNodeLabels() {
    if (!state.explorerCy) { explorerLabelLayer = null; return; }
    const layer = ensureExplorerLabelLayer();
    const width = graphContainer.clientWidth;
    const height = graphContainer.clientHeight;
    const viewportPadding = 140;
    const zoom = Number(state.explorerCy.zoom() || 1);
    const labelScale = Math.max(0.3, Math.min(1.35, zoom));
    const labelFontPx = 11.84 * labelScale;
    const liveFontPx = 10.88 * labelScale;
    const labelMaxWidthPx = Math.max(48, Math.min(220, 150 * labelScale));
    const labelGapPx = Math.max(2, 8 * labelScale);
    const html = [];
    state.explorerCy.nodes().forEach((node) => {
      const data = node.data();
      const displayLabel = String(data.displayLabel || "").trim();
      const liveHoldingsLabel = String(data.liveHoldingsLabel || "").trim();
      if (!displayLabel && !liveHoldingsLabel) return;
      const renderedPos = node.renderedPosition();
      if (!renderedPos) return;
      if (renderedPos.x < -viewportPadding || renderedPos.x > width + viewportPadding || renderedPos.y < -viewportPadding || renderedPos.y > height + viewportPadding) return;
      const rh = typeof node.renderedOuterHeight === "function" ? Number(node.renderedOuterHeight() || 0) : typeof node.renderedHeight === "function" ? Number(node.renderedHeight() || 0) : 0;
      const topY = renderedPos.y - rh / 2 - labelGapPx;
      const bottomY = renderedPos.y + rh / 2 + labelGapPx;
      if (displayLabel) {
        html.push(`<div class="graph-node-text graph-node-label" style="left:${renderedPos.x.toFixed(1)}px;top:${topY.toFixed(1)}px;font-size:${labelFontPx.toFixed(2)}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(displayLabel)}</div>`);
      }
      if (liveHoldingsLabel) {
        const uc = data.liveHoldingsStatus === "error" ? " is-unavailable" : "";
        html.push(`<div class="graph-node-text graph-node-live${uc}" style="left:${renderedPos.x.toFixed(1)}px;top:${bottomY.toFixed(1)}px;font-size:${liveFontPx.toFixed(2)}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(liveHoldingsLabel)}</div>`);
      }
    });
    layer.innerHTML = html.join("");
  }

  function scheduleExplorerLabelRender() {
    if (!state.explorerCy) { explorerLabelLayer = null; return; }
    if (explorerLabelFrame) return;
    explorerLabelFrame = window.requestAnimationFrame(() => {
      explorerLabelFrame = 0;
      renderExplorerNodeLabels();
    });
  }

  // --- Render graph ---
  function renderExplorerGraph(visible) {
    if (!visible.nodes.length) {
      clearExplorerGraphSurface();
      graphContainer.innerHTML = graphFiltersAreActive(state.explorerGraphFilters)
        ? `<div class="empty-state">No graph elements match the current filters. <button type="button" class="secondary" data-graph-reset-filters="explorer">Reset filters</button></div>`
        : `<div class="empty-state">No graphable flows found for this address.</div>`;
      return;
    }

    const elements = [
      ...visible.nodes.map((n) => ({ data: n })),
      ...visible.edges.map((e) => ({ data: e })),
    ];

    if (!state.explorerCy) {
      graphContainer.innerHTML = "";
      state.explorerCy = cytoscape({
        container: graphContainer,
        elements,
        style: explorerGraphStylesheet(),
        wheelSensitivity: 0.3,
        zoomingEnabled: true,
        userZoomingEnabled: false,
        boxSelectionEnabled: false,
        selectionType: "additive",
        userPanningEnabled: false,
        autoungrabify: false,
      });

      // Node tap / double-tap
      state.explorerCy.on("tap", "node", (event) => {
        if (graphTapSuppressed(graphContainer)) {
          return;
        }
        const data = event.target.data();
        const nodeID = String(event.target.id() || "");
        const now = Date.now();
        if (nodeID && nodeID === explorerLastTappedNodeID && now - explorerLastTappedAt <= nodeDoubleTapWindowMS) {
          if (explorerNodeTapTimer) { clearTimeout(explorerNodeTapTimer); explorerNodeTapTimer = null; }
          explorerLastTappedNodeID = "";
          explorerLastTappedAt = 0;
          void expandOneEdgeFromNode(data);
          return;
        }
        explorerLastTappedNodeID = nodeID;
        explorerLastTappedAt = now;
        if (explorerNodeTapTimer) { clearTimeout(explorerNodeTapTimer); explorerNodeTapTimer = null; }
        explorerNodeTapTimer = setTimeout(() => {
          explorerNodeTapTimer = null;
          explorerLastTappedNodeID = "";
          explorerLastTappedAt = 0;
          inspector.textContent = JSON.stringify(data.inspect, null, 2);
        }, nodeDoubleTapWindowMS);
      });

      // Context menu on node
      state.explorerCy.on("cxttap", "node", (event) => {
        const data = event.target.data();
        const rect = graphContainer.getBoundingClientRect();
        const rp = event.renderedPosition || event.target.renderedPosition();
        showExplorerContextMenu(data, { x: rect.left + rp.x, y: rect.top + rp.y });
      });

      // Context menu on background
      state.explorerCy.on("cxttap", (event) => {
        if (event.target !== state.explorerCy) return;
        if (Date.now() - explorerLastPaneContextMenuOpenedAt < 120) return;
        const rect = graphContainer.getBoundingClientRect();
        const rp = event.renderedPosition || { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 };
        showExplorerPaneContextMenu({ x: rect.left + rp.x, y: rect.top + rp.y });
      });

      state.explorerCy.on("tap", "edge", (event) => {
        if (graphTapSuppressed(graphContainer)) {
          return;
        }
        inspector.textContent = JSON.stringify(event.target.data().inspect, null, 2);
      });
      state.explorerCy.on("zoom pan", () => {
        state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
      });
      state.explorerCy.on("render zoom pan resize add remove data position", () => {
        scheduleExplorerLabelRender();
      });
    } else {
      state.explorerCy.elements().remove();
      state.explorerCy.add(elements);
    }

    explorerApplyElkLayout(state.explorerCy, visible.nodes)
      .then(() => {
        if (state.explorerViewport) {
          state.explorerCy.zoom(state.explorerViewport.zoom);
          state.explorerCy.pan(state.explorerViewport.pan);
        } else {
          state.explorerCy.fit(state.explorerCy.elements(), 40);
        }
        scheduleExplorerLabelRender();
      })
      .catch((err) => {
        inspector.textContent = `Layout failed: ${String(err)}`;
      });
  }

  // --- Expand one edge (one hop from neighbor node) ---
  function explorerNodeAddress(nodeData) {
    const addr = String((nodeData.metrics && nodeData.metrics.address) || "").trim();
    if (addr) return addr;
    const seeds = explorerAddressesForNodeExpansion(nodeData);
    return seeds.length === 1 ? seeds[0].address : "";
  }

  function explorerAddressesForNodeExpansion(nodeData) {
    if (!state.explorerGraph) return [];
    const rawNodeByID = new Map((state.explorerGraph.nodes || []).map((n) => [n.id, n]));
    const out = [];
    const rawNodeIDs = Array.isArray(nodeData.rawNodeIDs) ? nodeData.rawNodeIDs : [];
    rawNodeIDs.forEach((rawID) => {
      const rawNode = rawNodeByID.get(rawID);
      const candidate = rawNode && rawNode.metrics ? String(rawNode.metrics.address || "").trim() : "";
      if (candidate) out.push(buildFrontierSeed(candidate, rawNode?.chain));
    });
    const seen = new Map();
    out.forEach((seed) => { if (seed && seed.encoded && !seen.has(seed.encoded)) seen.set(seed.encoded, seed); });
    return Array.from(seen.values());
  }

  function explorerRawNodesForVisibleNode(nodeData) {
    if (!state.explorerGraph) return [];
    const rawNodeByID = new Map((state.explorerGraph.nodes || []).map((n) => [String(n.id), n]));
    const requestedIDs = Array.isArray(nodeData.rawNodeIDs) && nodeData.rawNodeIDs.length ? nodeData.rawNodeIDs.map((id) => String(id)) : [String(nodeData.id || "")];
    const seen = new Set();
    const out = [];
    requestedIDs.forEach((rawID) => {
      const rawNode = rawNodeByID.get(rawID);
      if (rawNode && !seen.has(rawID)) { seen.add(rawID); out.push(rawNode); }
    });
    return out;
  }

  async function expandOneEdgeFromNode(nodeData) {
    if (!state.explorerBaseGraph) return;
    if (state.explorerExpansionInFlight) return;

    const seeds = explorerAddressesForNodeExpansion(nodeData);
    if (!seeds.length) {
      inspector.textContent = "Selected node has no address context to expand.";
      return;
    }

    const newlyAddedKeys = [];
    seeds.forEach((seed) => {
      const key = addressKey(seed.encoded);
      if (!key || state.explorerExpandedHopAddressMap.has(key)) return;
      state.explorerExpandedHopAddressMap.set(key, seed.encoded);
      newlyAddedKeys.push(key);
    });
    if (!newlyAddedKeys.length) {
      inspector.textContent = "Already expanded from this node.";
      return;
    }

    state.explorerExpansionInFlight = true;
    if (state.explorerCy) {
      state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
    }
    inspector.textContent = `Expanding one edge from ${seeds.length} address(es)…`;

    try {
      const expansion = await callAPI("/api/actor-tracker/expand", {
        method: "POST",
        body: {
          actor_ids: [],
          start_time: new Date(0).toISOString(),
          end_time: new Date().toISOString(),
          flow_types: [...DEFAULT_FLOW_TYPES],
          min_usd: Number(minUSD.value || 0),
          collapse_external: false,
          display_mode: "combined",
          addresses: Array.from(state.explorerExpandedHopAddressMap.values()),
        },
      });
      state.explorerGraph = mergeExplorerGraphResponse(state.explorerBaseGraph, expansion);
      renderExplorerGraphResponse();
      inspector.textContent = `Expanded from ${state.explorerExpandedHopAddressMap.size} address seed(s).`;
    } catch (err) {
      newlyAddedKeys.forEach((key) => state.explorerExpandedHopAddressMap.delete(key));
      inspector.textContent = `Edge expansion failed: ${String(err)}`;
    } finally {
      state.explorerExpansionInFlight = false;
    }
  }

  // --- Live value refresh ---
  async function explorerRefreshLiveValueForNode(nodeData) {
    if (!state.explorerGraph) { inspector.textContent = "Explore an address first."; return; }
    const rawNodes = explorerRawNodesForVisibleNode(nodeData);
    if (!rawNodes.length) { inspector.textContent = "No live value context."; return; }
    if (state.explorerCy) state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
    inspector.textContent = `Refreshing live value for ${rawNodes.length} node(s)…`;
    try {
      const response = await callAPI("/api/actor-tracker/live-holdings", { method: "POST", body: { nodes: rawNodes } });
      const refreshedNodes = Array.isArray(response.nodes) ? response.nodes : [];
      const refreshedNodeByID = new Map(refreshedNodes.map((n) => [String(n.id), n]));
      function applyRefresh(graph) {
        if (!graph || !Array.isArray(graph.nodes)) return;
        graph.nodes = graph.nodes.map((n) => {
          const r = refreshedNodeByID.get(String(n.id));
          return r ? { ...n, metrics: { ...(n.metrics || {}), ...(r.metrics || {}) } } : n;
        });
      }
      applyRefresh(state.explorerBaseGraph);
      applyRefresh(state.explorerGraph);
      renderExplorerGraphResponse();
      inspector.textContent = `Refreshed ${refreshedNodes.length} node(s).`;
    } catch (err) {
      inspector.textContent = `Live value refresh failed: ${String(err)}`;
    }
  }

  async function explorerRefreshUnavailableNodes() {
    if (!state.explorerGraph) { inspector.textContent = "Explore an address first."; return; }
    const unavailable = (state.explorerGraph.nodes || []).filter((n) => {
      const s = String(n?.metrics?.live_holdings_status || "").trim().toLowerCase();
      return s === "error" || s === "unavailable";
    });
    if (!unavailable.length) { inspector.textContent = "No unavailable nodes."; return; }
    if (state.explorerCy) state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
    inspector.textContent = `Checking ${unavailable.length} unavailable node(s)…`;
    try {
      const response = await callAPI("/api/actor-tracker/live-holdings", { method: "POST", body: { nodes: unavailable } });
      const refreshedNodes = Array.isArray(response.nodes) ? response.nodes : [];
      const refreshedNodeByID = new Map(refreshedNodes.map((n) => [String(n.id), n]));
      function applyRefresh(graph) {
        if (!graph || !Array.isArray(graph.nodes)) return;
        graph.nodes = graph.nodes.map((n) => {
          const r = refreshedNodeByID.get(String(n.id));
          return r ? { ...n, metrics: { ...(n.metrics || {}), ...(r.metrics || {}) } } : n;
        });
      }
      applyRefresh(state.explorerBaseGraph);
      applyRefresh(state.explorerGraph);
      renderExplorerGraphResponse();
      inspector.textContent = `Checked ${unavailable.length}; refreshed ${refreshedNodes.length}.`;
    } catch (err) {
      inspector.textContent = `Unavailable check failed: ${String(err)}`;
    }
  }

  // --- Context menu for explorer graph ---
  function showExplorerContextMenu(nodeData, pagePos) {
    explorerContextMenuTarget = nodeData;
    explorerContextMenuMode = "node";
    // Swap "Expand one hop" label to "Expand one edge"
    const expandBtn = contextMenu.querySelector('[data-action="expand-hop"]');
    if (expandBtn) expandBtn.textContent = "Expand one edge";
    updateExplorerContextMenuButtons(explorerContextMenuMode);
    contextMenu.style.display = "block";
    const menuRect = contextMenu.getBoundingClientRect();
    contextMenu.style.left = `${Math.min(pagePos.x, window.innerWidth - menuRect.width - 8)}px`;
    contextMenu.style.top = `${Math.min(pagePos.y, window.innerHeight - menuRect.height - 8)}px`;
  }

  function showExplorerPaneContextMenu(pagePos) {
    explorerContextMenuTarget = null;
    explorerContextMenuMode = "pane";
    const expandBtn = contextMenu.querySelector('[data-action="expand-hop"]');
    if (expandBtn) expandBtn.textContent = "Expand one edge";
    updateExplorerContextMenuButtons(explorerContextMenuMode);
    contextMenu.style.display = "block";
    const menuRect = contextMenu.getBoundingClientRect();
    contextMenu.style.left = `${Math.min(pagePos.x, window.innerWidth - menuRect.width - 8)}px`;
    contextMenu.style.top = `${Math.min(pagePos.y, window.innerHeight - menuRect.height - 8)}px`;
    explorerLastPaneContextMenuOpenedAt = Date.now();
  }

  function updateExplorerContextMenuButtons(mode) {
    const nm = String(mode || "").trim().toLowerCase();
    contextMenu.querySelectorAll("button[data-action]").forEach((btn) => {
      const scopes = String(btn.dataset.scope || "node,pane").split(",").map((v) => v.trim().toLowerCase()).filter(Boolean);
      btn.style.display = scopes.includes(nm) ? "block" : "none";
    });
  }

  // Override the shared context menu click handler when explorer tab is active.
  contextMenu.addEventListener("click", async (e) => {
    if (!document.querySelector('[data-panel="address-explorer"].active')) return;
    const target = e.target instanceof Element ? e.target : null;
    if (!target) return;
    const button = target.closest("button[data-action]");
    if (!button) return;
    const action = button.dataset.action;
    const data = explorerContextMenuTarget;
    contextMenu.style.display = "none";
    explorerContextMenuTarget = null;

    if (action === "check-unavailable") {
      e.stopImmediatePropagation();
      await explorerRefreshUnavailableNodes();
      return;
    }
    if (!data) return;
    e.stopImmediatePropagation();
    const address = explorerNodeAddress(data);

    switch (action) {
      case "explorer": {
        const url = explorerURLForAddress(address, data.chain);
        if (url) window.open(url, "_blank");
        break;
      }
      case "copy-address":
        if (address) navigator.clipboard.writeText(address).then(() => { inspector.textContent = `Copied: ${address}`; });
        break;
      case "refresh-live-value":
        await explorerRefreshLiveValueForNode(data);
        break;
      case "expand-hop":
        void expandOneEdgeFromNode(data);
        break;
      case "label-node": {
        const label = prompt("Enter label for this node:", "");
        if (label !== null && address) {
          try {
            await callAPI("/api/address-annotations", { method: "PUT", body: { address, kind: "label", value: label } });
            await refreshSharedAnnotations();
            if (state.explorerCy) state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
            renderExplorerGraphResponse();
          } catch (err) { inspector.textContent = `Label failed: ${String(err)}`; }
        }
        break;
      }
      case "mark-asgard":
        if (address) {
          try {
            await callAPI("/api/address-annotations", { method: "PUT", body: { address, kind: "asgard_vault", value: "true" } });
            await refreshSharedAnnotations();
            if (state.explorerCy) state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
            renderExplorerGraphResponse();
          } catch (err) { inspector.textContent = `Mark Asgard failed: ${String(err)}`; }
        }
        break;
      case "remove-node":
        if (address) {
          try {
            await callAPI("/api/address-blocklist", { method: "POST", body: { address, reason: "Removed from graph" } });
            await refreshSharedAnnotations();
            if (state.explorerCy) state.explorerViewport = { zoom: state.explorerCy.zoom(), pan: state.explorerCy.pan() };
            renderExplorerGraphResponse();
          } catch (err) { inspector.textContent = `Remove failed: ${String(err)}`; }
        }
        break;
    }
  });

  // --- Keyboard shortcuts for explorer ---
  document.addEventListener("keydown", (e) => {
    const tag = document.activeElement?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;
    if (!document.querySelector('[data-panel="address-explorer"].active')) return;
    if (e.key === "Escape") {
      hideExplorerFilterPopover();
    }
    if (!state.explorerCy) return;
    switch (e.key) {
      case "+": case "=":
        e.preventDefault();
        state.explorerCy.zoom({ level: state.explorerCy.zoom() * 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
        break;
      case "-": case "_":
        e.preventDefault();
        state.explorerCy.zoom({ level: state.explorerCy.zoom() / 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
        break;
      case "0":
        e.preventDefault();
        state.explorerCy.fit(state.explorerCy.elements(), 40);
        state.explorerViewport = null;
        break;
      case "f": case "F":
        e.preventDefault();
        if (!graphCard) break;
        const isFs = graphCard.classList.toggle("fullscreen");
        document.body.style.overflow = isFs ? "hidden" : "";
        if (state.explorerCy) { state.explorerCy.resize(); if (!isFs) state.explorerCy.fit(state.explorerCy.elements(), 40); scheduleExplorerLabelRender(); }
        break;
    }
  });

  // --- Graph toolbar ---
  if (graphToolbar) {
    graphToolbar.addEventListener("click", (e) => {
      const button = e.target instanceof Element ? e.target.closest("button[data-graph-action]") : null;
      if (!button) return;
      if (button.dataset.graphAction === "filters") {
        toggleExplorerFilterPopover();
        return;
      }
      if (!state.explorerCy) return;
      switch (button.dataset.graphAction) {
        case "zoom-in":
          state.explorerCy.zoom({ level: state.explorerCy.zoom() * 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
          break;
        case "zoom-out":
          state.explorerCy.zoom({ level: state.explorerCy.zoom() / 1.3, renderedPosition: { x: graphContainer.clientWidth / 2, y: graphContainer.clientHeight / 2 } });
          break;
        case "fit":
          state.explorerCy.fit(state.explorerCy.elements(), 40);
          state.explorerViewport = null;
          break;
        case "fullscreen":
          if (!graphCard) break;
          const isFs = graphCard.classList.toggle("fullscreen");
          document.body.style.overflow = isFs ? "hidden" : "";
          if (state.explorerCy) { state.explorerCy.resize(); if (!isFs) state.explorerCy.fit(state.explorerCy.elements(), 40); scheduleExplorerLabelRender(); }
          break;
      }
    });
  }

  renderExplorerFilterPopover();
  renderExplorerMeta();
  updatePaginationBar();
  showExplorerPlaceholder("Enter a wallet address to explore.");
  void refreshExplorerRuns();
}

const activateTab = safeInit("bindTabs", bindTabs, () => {});
safeInit("bindHealth", bindHealth);
safeInit("bindActorTracker", () => bindActorTracker(activateTab, {
  lookup() {
    return Promise.resolve();
  },
}));
safeInit("bindAddressExplorer", () => bindAddressExplorer(activateTab));
