import {
  GRAPH_FILTER_TXN_TYPES,
  createGraphFilterState,
  setGraphFilterDateValue,
  setGraphFilterNumberValue,
  syncGraphFilterStateWithResponse,
  type GraphFilterState,
} from "./graph";
import type { ActorGraphResponse, AddressExplorerResponse } from "./types";

type FilterableGraph = Pick<ActorGraphResponse | AddressExplorerResponse, "nodes" | "edges">;

export interface SavedGraphCanvasPosition {
  x: number;
  y: number;
}

export interface SavedGraphCanvasViewport {
  zoom: number;
  pan: SavedGraphCanvasPosition;
}

export interface SavedGraphCanvasState {
  node_positions: Record<string, SavedGraphCanvasPosition>;
  viewport?: SavedGraphCanvasViewport;
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function readNumberArray(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is number => typeof item === "number" && Number.isFinite(item))
    : [];
}

export function readStringArray(value: unknown) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

export async function readJSONFile(file: File): Promise<unknown> {
  return JSON.parse(await file.text());
}

export function readSavedGraphCanvasState(value: unknown): SavedGraphCanvasState | null {
  if (!isRecord(value)) {
    return null;
  }

  const nodePositions: Record<string, SavedGraphCanvasPosition> = {};
  if (isRecord(value.node_positions)) {
    Object.entries(value.node_positions).forEach(([id, position]) => {
      if (!isRecord(position) || typeof position.x !== "number" || typeof position.y !== "number") {
        return;
      }
      if (!Number.isFinite(position.x) || !Number.isFinite(position.y)) {
        return;
      }
      nodePositions[id] = { x: position.x, y: position.y };
    });
  }

  let viewport: SavedGraphCanvasViewport | undefined;
  if (isRecord(value.viewport) && isRecord(value.viewport.pan)) {
    const { zoom, pan } = value.viewport;
    if (
      typeof zoom === "number" &&
      Number.isFinite(zoom) &&
      typeof pan.x === "number" &&
      Number.isFinite(pan.x) &&
      typeof pan.y === "number" &&
      Number.isFinite(pan.y)
    ) {
      viewport = {
        zoom,
        pan: {
          x: pan.x,
          y: pan.y,
        },
      };
    }
  }

  if (!Object.keys(nodePositions).length && !viewport) {
    return null;
  }

  return {
    node_positions: nodePositions,
    viewport,
  };
}

export function restoreSavedGraphFilters(savedFilters: unknown, graph: FilterableGraph): GraphFilterState {
  const next = createGraphFilterState();
  syncGraphFilterStateWithResponse(next, graph, { reset: true });

  if (!isRecord(savedFilters)) {
    return next;
  }

  const txnTypes = savedFilters.txnTypes;
  if (isRecord(txnTypes)) {
    GRAPH_FILTER_TXN_TYPES.forEach((item) => {
      const enabled = txnTypes[item.key];
      if (typeof enabled === "boolean") {
        next.txnTypes[item.key] = enabled;
      }
    });
  }

  const selectedChains = readStringArray(savedFilters.selectedChains).filter((chain) => next.availableChains.includes(chain));
  if (selectedChains.length) {
    next.selectedChains = selectedChains;
  }

  if (typeof savedFilters.startTime === "string") {
    setGraphFilterDateValue(next, "startTime", savedFilters.startTime);
  }
  if (typeof savedFilters.endTime === "string") {
    setGraphFilterDateValue(next, "endTime", savedFilters.endTime);
  }
  if (savedFilters.minTxnUSD === null || typeof savedFilters.minTxnUSD === "number" || typeof savedFilters.minTxnUSD === "string") {
    setGraphFilterNumberValue(next, "minTxnUSD", savedFilters.minTxnUSD === null ? "" : String(savedFilters.minTxnUSD));
  }
  if (savedFilters.maxTxnUSD === null || typeof savedFilters.maxTxnUSD === "number" || typeof savedFilters.maxTxnUSD === "string") {
    setGraphFilterNumberValue(next, "maxTxnUSD", savedFilters.maxTxnUSD === null ? "" : String(savedFilters.maxTxnUSD));
  }

  next.initialized = true;
  next.isOpen = false;
  return next;
}
