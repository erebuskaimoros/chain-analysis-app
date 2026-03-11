import type { ActorGraphResponse, AddressExplorerResponse, SupportingAction } from "../types";
import { GRAPH_FILTER_TXN_TYPES, type GraphFilterState } from "./types";
import {
  chainSelectionsMatchAll,
  clampGraphFilterNumber,
  clampISOToRange,
  graphChainsAllowed,
  graphFilterMetadataFromResponse,
  graphItemChainSet,
  graphTxnTypeAllowed,
  normalizeGraphFilterNumber,
  normalizeISODateTime,
  timeSelectionsMatchFullRange,
  uniqueStrings,
  valueSelectionsMatchFullRange,
} from "./internals";

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
