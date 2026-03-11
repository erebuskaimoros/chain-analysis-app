import { useEffect, useRef, useState } from "react";
import type cytoscape from "cytoscape";
import { GraphCanvasOverlays } from "./graph-canvas/GraphCanvasOverlays";
import { useGraphCanvasCore } from "./graph-canvas/useGraphCanvasCore";
import { useGraphCanvasInteractions } from "./graph-canvas/useGraphCanvasInteractions";
import { useGraphLabelLayer } from "./graph-canvas/useGraphLabelLayer";
import type { ContextMenuState, GraphCanvasProps } from "./graph-canvas/types";

export function GraphCanvas({
  mode,
  nodes,
  edges,
  selection,
  onSelectionChange,
  onNodePrimaryAction,
  onNodeDoubleActivate,
  doubleActivateLabel = mode === "explorer" ? "Expand one edge" : "Expand one hop",
  graphResetKey = 0,
  onSaveState,
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
  const cyRef = useRef<cytoscape.Core | null>(null);
  const viewportRef = useRef<{ zoom: number; pan: cytoscape.Position } | null>(null);
  const suppressTapUntilRef = useRef(0);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [menuState, setMenuState] = useState<ContextMenuState>(null);

  const { labelLayerRef, scheduleLabelRender, cancelScheduledLabelRender } = useGraphLabelLayer(cyRef, surfaceRef);

  useGraphCanvasCore({
    mode,
    nodes,
    edges,
    selection,
    onSelectionChange,
    onNodePrimaryAction,
    onNodeDoubleActivate,
    graphResetKey,
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
    onNodeDoubleActivate,
    onSaveState,
    scheduleLabelRender,
  });

  useEffect(() => {
    scheduleLabelRender();
  }, [isFullscreen, scheduleLabelRender]);

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
            onToolbarAction={handleToolbarAction}
            onContextMenuAction={handleContextMenuAction}
          />
        </div>
      </div>
    </div>
  );
}
