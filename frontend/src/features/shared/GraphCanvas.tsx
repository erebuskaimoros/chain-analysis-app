import { useEffect, useRef, useState } from "react";
import type cytoscape from "cytoscape";
import { GraphCanvasOverlays } from "./graph-canvas/GraphCanvasOverlays";
import { useGraphCanvasCore } from "./graph-canvas/useGraphCanvasCore";
import { useGraphCanvasInteractions } from "./graph-canvas/useGraphCanvasInteractions";
import { useGraphHoverCard } from "./graph-canvas/useGraphHoverCard";
import { useGraphLabelLayer } from "./graph-canvas/useGraphLabelLayer";
import { useGraphMinimap } from "./graph-canvas/useGraphMinimap";
import { useGraphNeighborhoodHighlight } from "./graph-canvas/useGraphNeighborhoodHighlight";
import { useGraphSearch } from "./graph-canvas/useGraphSearch";
import type { ContextMenuState, GraphCanvasProps, GraphWheelMode } from "./graph-canvas/types";

const WHEEL_MODE_STORAGE_KEY = "graph-canvas-wheel-mode";

function readStoredWheelMode(): GraphWheelMode {
  try {
    const stored = window.localStorage.getItem(WHEEL_MODE_STORAGE_KEY);
    return stored === "zoom" || stored === "pan" ? stored : "auto";
  } catch {
    return "auto";
  }
}

function nextWheelMode(mode: GraphWheelMode): GraphWheelMode {
  return mode === "auto" ? "zoom" : mode === "zoom" ? "pan" : "auto";
}

export function GraphCanvas({
  mode,
  nodes,
  edges,
  selection,
  onSelectionChange,
  onNodePrimaryAction,
  nodeHasPrimaryAction,
  onNodeDoubleActivate,
  doubleActivateLabel = "Expand one edge",
  graphResetKey = 0,
  onSaveState,
  savedCanvasState,
  onFullscreenChange,
  filters,
  nodeMenuActions,
  paneMenuActions,
}: GraphCanvasProps) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const cyMountRef = useRef<HTMLDivElement | null>(null);
  const selectionBoxRef = useRef<HTMLDivElement | null>(null);
  const filterPopoverRef = useRef<HTMLDivElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const minimapCanvasRef = useRef<HTMLCanvasElement | null>(null);
  const cyRef = useRef<cytoscape.Core | null>(null);
  const viewportRef = useRef<{ zoom: number; pan: cytoscape.Position } | null>(null);
  const suppressTapUntilRef = useRef(0);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [menuState, setMenuState] = useState<ContextMenuState>(null);
  const [wheelMode, setWheelMode] = useState<GraphWheelMode>(readStoredWheelMode);

  const { labelLayerRef, scheduleLabelRender, cancelScheduledLabelRender } = useGraphLabelLayer(cyRef, surfaceRef);

  useGraphCanvasCore({
    mode,
    nodes,
    edges,
    selection,
    onSelectionChange,
    onNodePrimaryAction,
    nodeHasPrimaryAction,
    onNodeDoubleActivate,
    graphResetKey,
    savedCanvasState,
    cyRef,
    viewportRef,
    suppressTapUntilRef,
    surfaceRef,
    cyMountRef,
    scheduleLabelRender,
    cancelScheduledLabelRender,
  });

  const { handleToolbarAction, handleContextMenuAction } = useGraphCanvasInteractions({
    cyRef,
    viewportRef,
    suppressTapUntilRef,
    rootRef,
    surfaceRef,
    selectionBoxRef,
    filterPopoverRef,
    menuRef,
    selection,
    onSelectionChange,
    filters,
    nodeMenuActions,
    paneMenuActions,
    menuState,
    setMenuState,
    isFullscreen,
    setIsFullscreen,
    wheelMode,
    onNodeDoubleActivate,
    onSaveState,
    scheduleLabelRender,
  });

  const search = useGraphSearch(cyRef, nodes, scheduleLabelRender);
  const hoverCard = useGraphHoverCard(cyRef, surfaceRef);
  useGraphNeighborhoodHighlight(cyRef, selection, scheduleLabelRender);
  useGraphMinimap(cyRef, minimapCanvasRef, surfaceRef);

  useEffect(() => {
    try {
      window.localStorage.setItem(WHEEL_MODE_STORAGE_KEY, wheelMode);
    } catch {
      // localStorage unavailable (private browsing); preference stays session-only.
    }
  }, [wheelMode]);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "/") {
        return;
      }
      const tag = document.activeElement?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") {
        return;
      }
      event.preventDefault();
      searchInputRef.current?.focus();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
    };
  }, []);

  useEffect(() => {
    scheduleLabelRender();
  }, [isFullscreen, scheduleLabelRender]);

  useEffect(() => {
    onFullscreenChange?.(isFullscreen);
  }, [isFullscreen, onFullscreenChange]);

  useEffect(
    () => () => {
      onFullscreenChange?.(false);
    },
    [onFullscreenChange]
  );

  return (
    <div className="graph-frame" ref={rootRef}>
      <div className="graph-container">
        <div className="graph-surface" ref={surfaceRef}>
          <div className="graph-canvas" ref={cyMountRef} />
          <div className="graph-label-layer" ref={labelLayerRef} />
          <div className="graph-selection-box" ref={selectionBoxRef} />

          <GraphCanvasOverlays
            filters={filters}
            filterPopoverRef={filterPopoverRef}
            menuRef={menuRef}
            menuState={menuState}
            nodeMenuActions={nodeMenuActions}
            paneMenuActions={paneMenuActions}
            doubleActivateLabel={doubleActivateLabel}
            showSaveState={Boolean(onSaveState)}
            search={search}
            searchInputRef={searchInputRef}
            wheelMode={wheelMode}
            onCycleWheelMode={() => setWheelMode((current) => nextWheelMode(current))}
            hoverCard={hoverCard}
            minimapCanvasRef={minimapCanvasRef}
            onToolbarAction={handleToolbarAction}
            onContextMenuAction={handleContextMenuAction}
          />
        </div>
      </div>
    </div>
  );
}
