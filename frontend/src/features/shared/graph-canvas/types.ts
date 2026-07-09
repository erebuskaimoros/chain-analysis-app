import type { ReactNode } from "react";
import type { GraphSelection, VisibleGraphEdge, VisibleGraphNode } from "../../../lib/graph";
import type { SavedGraphCanvasState } from "../../../lib/graphState";

export type GraphWheelMode = "auto" | "zoom" | "pan";

export interface GraphCanvasFilters {
  isOpen: boolean;
  isActive: boolean;
  content: ReactNode;
  onToggle: () => void;
  onClose: () => void;
}

export interface GraphCanvasNodeMenuActions {
  onOpenExplorer?: (node: VisibleGraphNode) => void;
  onCopyAddress?: (node: VisibleGraphNode) => void;
  onRefreshLiveValue?: (node: VisibleGraphNode) => void;
  onExpandNodes?: (nodes: VisibleGraphNode[]) => void;
  onLabelNode?: (node: VisibleGraphNode) => void;
  onMarkAsgard?: (node: VisibleGraphNode) => void;
  onRemoveNode?: (node: VisibleGraphNode) => void;
}

export interface GraphCanvasPaneMenuActions {
  onCheckUnavailable?: () => void;
}

export interface GraphCanvasProps {
  mode: "actor" | "explorer";
  nodes: VisibleGraphNode[];
  edges: VisibleGraphEdge[];
  selection: GraphSelection;
  onSelectionChange: (selection: GraphSelection) => void;
  onNodePrimaryAction?: (node: VisibleGraphNode) => boolean;
  /**
   * Predicate for nodes whose single-tap triggers a primary action. Taps on
   * those nodes are deferred by the double-tap window; all other nodes select
   * immediately. When omitted and onNodePrimaryAction is set, every node is
   * treated as having a primary action (deferred).
   */
  nodeHasPrimaryAction?: (node: VisibleGraphNode) => boolean;
  onNodeDoubleActivate?: (node: VisibleGraphNode) => void;
  doubleActivateLabel?: string;
  graphResetKey?: number;
  onSaveState?: (canvasState: SavedGraphCanvasState) => void;
  savedCanvasState?: SavedGraphCanvasState | null;
  onFullscreenChange?: (isFullscreen: boolean) => void;
  filters?: GraphCanvasFilters;
  nodeMenuActions?: GraphCanvasNodeMenuActions;
  paneMenuActions?: GraphCanvasPaneMenuActions;
}

export type ContextMenuState =
  | { mode: "node"; node: VisibleGraphNode; x: number; y: number }
  | { mode: "nodes"; nodes: VisibleGraphNode[]; x: number; y: number }
  | { mode: "pane"; x: number; y: number }
  | null;

export interface GraphSearchState {
  query: string;
  matchCount: number;
  activeIndex: number;
}

export interface GraphHoverCardState {
  node: VisibleGraphNode;
  x: number;
  y: number;
}
