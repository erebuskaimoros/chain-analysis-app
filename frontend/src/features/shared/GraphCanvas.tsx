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
import type { GraphLayoutPhase } from "./graph-canvas/useGraphCanvasCore";
import type { SavedGraphCanvasState } from "../../lib/graphState";

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
  defaultSaveName,
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
  const [pendingSaveState, setPendingSaveState] = useState<SavedGraphCanvasState | null>(null);
  const [layoutPhase, setLayoutPhase] = useState<GraphLayoutPhase | null>(null);

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
    onLayoutPhase: (phase) =>
      // Small incremental updates after the first layout should not hide the
      // "zoomed to the start" note.
      setLayoutPhase((current) =>
        phase.phase === "done" && !phase.focused && current?.phase === "done" && current.focused ? current : phase
      ),
  });

  // The "zoomed to the start" note fades after a while or on the next fit.
  useEffect(() => {
    if (layoutPhase?.phase !== "done" || !layoutPhase.focused) {
      return;
    }
    const timer = window.setTimeout(() => setLayoutPhase(null), 9000);
    return () => window.clearTimeout(timer);
  }, [layoutPhase]);

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
    onSaveRequest: (canvasState) => setPendingSaveState(canvasState),
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

  // The canvas fills a flexible area (panels open and close around it), so
  // follow its size and keep whatever was centred in the middle.
  useEffect(() => {
    const surface = surfaceRef.current;
    if (!surface || typeof ResizeObserver === "undefined") {
      return;
    }
    let previous = { width: surface.clientWidth, height: surface.clientHeight };
    let frame = 0;
    const observer = new ResizeObserver(() => {
      window.cancelAnimationFrame(frame);
      frame = window.requestAnimationFrame(() => {
        const cy = cyRef.current;
        const next = { width: surface.clientWidth, height: surface.clientHeight };
        if (!cy || (next.width === previous.width && next.height === previous.height)) {
          previous = next;
          return;
        }
        // Panning here fires cytoscape's viewport events, which would record
        // this adjustment as the user's own viewport. When no viewport is
        // recorded yet (a layout is about to fit the graph), keep it that way.
        const recorded = viewportRef.current;
        cy.resize();
        const dx = (next.width - previous.width) / 2;
        const dy = (next.height - previous.height) / 2;
        if (previous.width && previous.height && (dx || dy)) {
          const pan = cy.pan();
          cy.pan({ x: pan.x + dx, y: pan.y + dy });
        }
        viewportRef.current = recorded ? { zoom: cy.zoom(), pan: cy.pan() } : null;
        previous = next;
        scheduleLabelRender();
      });
    });
    observer.observe(surface);
    return () => {
      observer.disconnect();
      window.cancelAnimationFrame(frame);
    };
  }, [scheduleLabelRender]);

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
            savePrompt={pendingSaveState ? { defaultName: defaultSaveName ?? "graph state" } : null}
            onSaveConfirm={(name) => {
              if (pendingSaveState) {
                onSaveState?.(pendingSaveState, name);
              }
              setPendingSaveState(null);
            }}
            onSaveCancel={() => setPendingSaveState(null)}
            search={search}
            searchInputRef={searchInputRef}
            wheelMode={wheelMode}
            onCycleWheelMode={() => setWheelMode((current) => nextWheelMode(current))}
            hoverCard={hoverCard}
            minimapCanvasRef={minimapCanvasRef}
            onToolbarAction={(action) => {
              if (action === "fit") {
                setLayoutPhase(null);
              }
              handleToolbarAction(action);
            }}
            onContextMenuAction={handleContextMenuAction}
          />
          {layoutPhase?.phase === "busy" && layoutPhase.nodeCount > 150 ? (
            <div className="graph-layout-note" role="status">
              <span className="spinner" aria-hidden="true" />
              Arranging {layoutPhase.nodeCount.toLocaleString()} nodes…
            </div>
          ) : null}
          {layoutPhase?.phase === "done" && layoutPhase.focused ? (
            <div className="graph-layout-note" role="status">
              Showing the starting addresses up close. Press 0 or use Fit to see all{" "}
              {layoutPhase.nodeCount.toLocaleString()} nodes.
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
