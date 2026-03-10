import { useEffect, useRef, useState, type ReactNode } from "react";
import cytoscape from "cytoscape";
import ELK from "elkjs/lib/elk.bundled.js";
import {
  graphLayoutNodeSize,
  graphStylesheet,
  type GraphSelection,
  type VisibleGraphEdge,
  type VisibleGraphNode,
} from "../../lib/graph";

interface GraphCanvasProps {
  mode: "actor" | "explorer";
  nodes: VisibleGraphNode[];
  edges: VisibleGraphEdge[];
  selection: GraphSelection;
  onSelectionChange: (selection: GraphSelection) => void;
  onNodePrimaryAction?: (node: VisibleGraphNode) => boolean;
  onNodeDoubleActivate?: (node: VisibleGraphNode) => void;
  doubleActivateLabel?: string;
  graphResetKey?: number;
  filterUI?: {
    isOpen: boolean;
    isActive: boolean;
    popover: ReactNode;
    onToggle: () => void;
    onClose: () => void;
  };
  onOpenExplorer?: (node: VisibleGraphNode) => void;
  onCopyAddress?: (node: VisibleGraphNode) => void;
  onRefreshLiveValue?: (node: VisibleGraphNode) => void;
  onLabelNode?: (node: VisibleGraphNode) => void;
  onMarkAsgard?: (node: VisibleGraphNode) => void;
  onRemoveNode?: (node: VisibleGraphNode) => void;
  onCheckUnavailable?: () => void;
}

type ContextMenuState =
  | { mode: "node"; node: VisibleGraphNode; x: number; y: number }
  | { mode: "pane"; x: number; y: number }
  | null;

const DOUBLE_TAP_WINDOW_MS = 320;

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
  filterUI,
  onOpenExplorer,
  onCopyAddress,
  onRefreshLiveValue,
  onLabelNode,
  onMarkAsgard,
  onRemoveNode,
  onCheckUnavailable,
}: GraphCanvasProps) {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const cyMountRef = useRef<HTMLDivElement | null>(null);
  const labelLayerRef = useRef<HTMLDivElement | null>(null);
  const selectionBoxRef = useRef<HTMLDivElement | null>(null);
  const filterPopoverRef = useRef<HTMLDivElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const cyRef = useRef<cytoscape.Core | null>(null);
  const elkRef = useRef(new ELK());
  const viewportRef = useRef<{ zoom: number; pan: cytoscape.Position } | null>(null);
  const layoutSeqRef = useRef(0);
  const suppressTapUntilRef = useRef(0);
  const lastTapRef = useRef<{ id: string; at: number }>({ id: "", at: 0 });
  const nodeTapTimerRef = useRef<number | null>(null);
  const labelFrameRef = useRef<number | null>(null);
  const lastPaneContextMenuOpenedAtRef = useRef(0);
  const selectionRef = useRef(selection);
  const selectionChangeRef = useRef(onSelectionChange);
  const nodePrimaryActionRef = useRef(onNodePrimaryAction);
  const nodeDoubleActivateRef = useRef(onNodeDoubleActivate);
  const nodeMapRef = useRef(new Map<string, VisibleGraphNode>());
  const edgeMapRef = useRef(new Map<string, VisibleGraphEdge>());
  const resetKeyRef = useRef(graphResetKey);

  const [isFullscreen, setIsFullscreen] = useState(false);
  const [menuState, setMenuState] = useState<ContextMenuState>(null);

  selectionRef.current = selection;
  selectionChangeRef.current = onSelectionChange;
  nodePrimaryActionRef.current = onNodePrimaryAction;
  nodeDoubleActivateRef.current = onNodeDoubleActivate;
  nodeMapRef.current = new Map(nodes.map((node) => [node.id, node]));
  edgeMapRef.current = new Map(edges.map((edge) => [edge.id, edge]));

  useEffect(() => {
    if (!cyMountRef.current) {
      return;
    }

    const cy = cytoscape({
      container: cyMountRef.current,
      elements: [],
      style: graphStylesheet(mode),
      wheelSensitivity: 0.3,
      zoomingEnabled: true,
      userZoomingEnabled: false,
      boxSelectionEnabled: false,
      selectionType: "additive",
      userPanningEnabled: false,
      autoungrabify: false,
    });

    function clearPendingNodeTap() {
      if (nodeTapTimerRef.current !== null) {
        window.clearTimeout(nodeTapTimerRef.current);
        nodeTapTimerRef.current = null;
      }
    }

    function graphTapSuppressed() {
      return Date.now() < suppressTapUntilRef.current;
    }

    cy.on("tap", "node", (event) => {
      if (graphTapSuppressed()) {
        return;
      }
      const tapped = nodeMapRef.current.get(event.target.id());
      if (!tapped) {
        return;
      }
      const now = Date.now();
      if (
        lastTapRef.current.id === tapped.id &&
        now - lastTapRef.current.at <= DOUBLE_TAP_WINDOW_MS &&
        nodeDoubleActivateRef.current
      ) {
        clearPendingNodeTap();
        lastTapRef.current = { id: "", at: 0 };
        nodeDoubleActivateRef.current(tapped);
        return;
      }

      lastTapRef.current = { id: tapped.id, at: now };
      clearPendingNodeTap();
      nodeTapTimerRef.current = window.setTimeout(() => {
        nodeTapTimerRef.current = null;
        lastTapRef.current = { id: "", at: 0 };
        if (nodePrimaryActionRef.current?.(tapped)) {
          return;
        }
        selectionChangeRef.current({ kind: "node", node: tapped });
      }, DOUBLE_TAP_WINDOW_MS);
    });

    cy.on("tap", "edge", (event) => {
      if (graphTapSuppressed()) {
        return;
      }
      const tapped = edgeMapRef.current.get(event.target.id());
      if (!tapped) {
        return;
      }
      selectionChangeRef.current({ kind: "edge", edge: tapped });
    });

    cy.on("tap", (event) => {
      if (event.target === cy && !graphTapSuppressed()) {
        selectionChangeRef.current(null);
      }
    });

    cy.on("cxttap", "node", (event) => {
      const node = nodeMapRef.current.get(event.target.id());
      const surface = surfaceRef.current;
      if (!node || !surface) {
        return;
      }
      const rect = surface.getBoundingClientRect();
      const renderedPosition = event.renderedPosition || event.target.renderedPosition();
      setMenuState({
        mode: "node",
        node,
        x: rect.left + renderedPosition.x,
        y: rect.top + renderedPosition.y,
      });
    });

    cy.on("cxttap", (event) => {
      if (event.target !== cy || !surfaceRef.current) {
        return;
      }
      if (Date.now() - lastPaneContextMenuOpenedAtRef.current < 120) {
        return;
      }
      const rect = surfaceRef.current.getBoundingClientRect();
      const renderedPosition = event.renderedPosition || {
        x: surfaceRef.current.clientWidth / 2,
        y: surfaceRef.current.clientHeight / 2,
      };
      setMenuState({
        mode: "pane",
        x: rect.left + renderedPosition.x,
        y: rect.top + renderedPosition.y,
      });
      lastPaneContextMenuOpenedAtRef.current = Date.now();
    });

    cy.on("zoom pan", () => {
      viewportRef.current = { zoom: cy.zoom(), pan: cy.pan() };
    });
    cy.on("render zoom pan resize add remove data position", () => {
      scheduleLabelRender();
    });

    cyRef.current = cy;

    return () => {
      clearPendingNodeTap();
      if (labelFrameRef.current !== null) {
        window.cancelAnimationFrame(labelFrameRef.current);
        labelFrameRef.current = null;
      }
      cy.destroy();
      cyRef.current = null;
    };
  }, [mode]);

  useEffect(() => {
    const cy = cyRef.current;
    const surface = surfaceRef.current;
    const box = selectionBoxRef.current;
    if (!cy || !surface || !box) {
      return;
    }
    const cyInstance = cy;
    const surfaceElement = surface;
    const selectionBox = box;

    let middlePanning = false;
    let panStart = { x: 0, y: 0 };
    let panOrigin = { x: 0, y: 0 };
    let boxSelecting = false;
    let boxDragged = false;
    let preserveSelection = false;
    let boxStart = { x: 0, y: 0 };
    let boxCurrent = { x: 0, y: 0 };

    function renderedSelectionRect() {
      const rect = surfaceElement.getBoundingClientRect();
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
      const rect = renderedSelectionRect();
      selectionBox.style.display = boxDragged ? "block" : "none";
      selectionBox.style.left = `${rect.x1}px`;
      selectionBox.style.top = `${rect.y1}px`;
      selectionBox.style.width = `${rect.width}px`;
      selectionBox.style.height = `${rect.height}px`;
    }

    function nodeIntersectsSelection(node: cytoscape.NodeSingular, rect: ReturnType<typeof renderedSelectionRect>) {
      const boundingBox = node.renderedBoundingBox({
        includeLabels: false,
        includeOverlays: false,
      });
      return (
        rect.x1 <= boundingBox.x2 &&
        rect.x2 >= boundingBox.x1 &&
        rect.y1 <= boundingBox.y2 &&
        rect.y2 >= boundingBox.y1
      );
    }

    function applyBoxSelection() {
      const rect = renderedSelectionRect();
      cyInstance.batch(() => {
        if (!preserveSelection) {
          cyInstance.elements(":selected").unselect();
        }
        cyInstance.nodes().forEach((node) => {
          if (node.visible() && nodeIntersectsSelection(node, rect)) {
            node.select();
          }
        });
      });
    }

    function stopBoxSelection() {
      boxSelecting = false;
      boxDragged = false;
      selectionBox.style.display = "none";
    }

    function hitNodeAtClientPoint(clientX: number, clientY: number) {
      return graphNodeAtClientPoint(cyInstance, surfaceElement, clientX, clientY);
    }

    function onMouseDown(event: MouseEvent) {
      if (event.button === 1) {
        middlePanning = true;
        panStart = { x: event.clientX, y: event.clientY };
        panOrigin = { ...cyInstance.pan() };
        event.preventDefault();
        return;
      }
      if (event.button !== 0) {
        return;
      }
      if (hitNodeAtClientPoint(event.clientX, event.clientY)) {
        return;
      }
      boxSelecting = true;
      boxDragged = false;
      preserveSelection = event.shiftKey || event.metaKey || event.ctrlKey;
      boxStart = { x: event.clientX, y: event.clientY };
      boxCurrent = { ...boxStart };
      selectionBox.style.display = "none";
      event.preventDefault();
    }

    function onMouseMove(event: MouseEvent) {
      if (middlePanning) {
        cyInstance.pan({
          x: panOrigin.x + (event.clientX - panStart.x),
          y: panOrigin.y + (event.clientY - panStart.y),
        });
        return;
      }
      if (!boxSelecting) {
        return;
      }
      boxCurrent = { x: event.clientX, y: event.clientY };
      if (!boxDragged) {
        const moved = Math.abs(boxCurrent.x - boxStart.x) + Math.abs(boxCurrent.y - boxStart.y);
        if (moved < 6) {
          return;
        }
        boxDragged = true;
      }
      updateSelectionBox();
      applyBoxSelection();
      event.preventDefault();
    }

    function onMouseUp(event: MouseEvent) {
      if (event.button === 1) {
        middlePanning = false;
      }
      if (event.button !== 0 || !boxSelecting) {
        return;
      }
      if (boxDragged) {
        applyBoxSelection();
        suppressTapUntilRef.current = Date.now() + 160;
        event.preventDefault();
        event.stopPropagation();
      }
      stopBoxSelection();
    }

    function onMouseLeave() {
      middlePanning = false;
      stopBoxSelection();
    }

    function onAuxClick(event: MouseEvent) {
      if (event.button === 1) {
        event.preventDefault();
      }
    }

    function onWheel(event: WheelEvent) {
      const rect = surfaceElement.getBoundingClientRect();
      const renderedPosition = {
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      };
      const currentZoom = Number(cyInstance.zoom() || 1);
      const zoomFactor = Math.exp(-event.deltaY * 0.0015);
      const minZoom = typeof cyInstance.minZoom === "function" ? Number(cyInstance.minZoom() || 0.05) : 0.05;
      const maxZoom = typeof cyInstance.maxZoom === "function" ? Number(cyInstance.maxZoom() || 10) : 10;
      cyInstance.zoom({
        level: clamp(currentZoom * zoomFactor, minZoom, maxZoom),
        renderedPosition,
      });
      event.preventDefault();
    }

    function onNativeContextMenu(event: MouseEvent) {
      event.preventDefault();
      if (hitNodeAtClientPoint(event.clientX, event.clientY)) {
        return;
      }
      setMenuState({ mode: "pane", x: event.clientX, y: event.clientY });
      lastPaneContextMenuOpenedAtRef.current = Date.now();
    }

    surfaceElement.addEventListener("mousedown", onMouseDown);
    surfaceElement.addEventListener("mousemove", onMouseMove);
    surfaceElement.addEventListener("mouseup", onMouseUp);
    surfaceElement.addEventListener("mouseleave", onMouseLeave);
    surfaceElement.addEventListener("auxclick", onAuxClick);
    surfaceElement.addEventListener("wheel", onWheel, { passive: false });
    surfaceElement.addEventListener("contextmenu", onNativeContextMenu);

    return () => {
      surfaceElement.removeEventListener("mousedown", onMouseDown);
      surfaceElement.removeEventListener("mousemove", onMouseMove);
      surfaceElement.removeEventListener("mouseup", onMouseUp);
      surfaceElement.removeEventListener("mouseleave", onMouseLeave);
      surfaceElement.removeEventListener("auxclick", onAuxClick);
      surfaceElement.removeEventListener("wheel", onWheel);
      surfaceElement.removeEventListener("contextmenu", onNativeContextMenu);
    };
  }, []);

  useEffect(() => {
    const surface = surfaceRef.current;
    if (!surface) {
      return;
    }
    const surfaceElement = surface;

    function onKeyDown(event: KeyboardEvent) {
      const tag = document.activeElement?.tagName;
      if (event.key === "Escape") {
        setMenuState(null);
        filterUI?.onClose();
        setIsFullscreen(false);
        return;
      }
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") {
        return;
      }
      const cy = cyRef.current;
      if (!cy) {
        return;
      }
      const center = { x: surfaceElement.clientWidth / 2, y: surfaceElement.clientHeight / 2 };
      switch (event.key) {
        case "+":
        case "=":
          event.preventDefault();
          cy.zoom({ level: cy.zoom() * 1.3, renderedPosition: center });
          break;
        case "-":
        case "_":
          event.preventDefault();
          cy.zoom({ level: cy.zoom() / 1.3, renderedPosition: center });
          break;
        case "0":
          event.preventDefault();
          viewportRef.current = null;
          cy.fit(cy.elements(), 40);
          scheduleLabelRender();
          break;
        case "f":
        case "F":
          event.preventDefault();
          setIsFullscreen((current) => !current);
          break;
      }
    }

    function onDocumentMouseDown(event: MouseEvent) {
      const target = event.target instanceof Node ? event.target : null;
      const targetElement = event.target instanceof Element ? event.target : null;
      if (menuRef.current && target && !menuRef.current.contains(target)) {
        setMenuState(null);
      }
      if (
        filterUI?.isOpen &&
        target &&
        filterPopoverRef.current &&
        !filterPopoverRef.current.contains(target) &&
        !targetElement?.closest(".graph-toolbar")
      ) {
        filterUI.onClose();
      }
    }

    window.addEventListener("keydown", onKeyDown);
    document.addEventListener("mousedown", onDocumentMouseDown);

    return () => {
      window.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("mousedown", onDocumentMouseDown);
    };
  }, [filterUI]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) {
      return;
    }

    if (resetKeyRef.current !== graphResetKey) {
      resetKeyRef.current = graphResetKey;
      viewportRef.current = null;
    }

    const elements: cytoscape.ElementDefinition[] = [
      ...nodes.map((node) => ({
        data: {
          ...node,
          id: node.id,
          kind: node.kind,
          depth: node.depth,
        },
      })),
      ...edges.map((edge) => ({
        data: {
          ...edge,
          id: edge.id,
          source: edge.source,
          target: edge.target,
          actionClass: edge.action_class,
          edgeLabel: edge.edgeLabel,
          lineColor: edge.lineColor,
          width: edge.width,
        },
      })),
    ];

    cy.batch(() => {
      cy.elements().remove();
      cy.add(elements);
    });

    const currentLayoutSeq = ++layoutSeqRef.current;
    void applyElkLayout(cy, mode, nodes, elkRef.current).then(() => {
      if (layoutSeqRef.current !== currentLayoutSeq || !cyRef.current) {
        return;
      }
      const viewport = viewportRef.current;
      if (viewport) {
        cy.zoom(viewport.zoom);
        cy.pan(viewport.pan);
      } else {
        cy.fit(cy.elements(), 40);
      }
      scheduleLabelRender();
    });

    const selected = selectionRef.current;
    if (selected) {
      const exists =
        selected.kind === "node"
          ? nodeMapRef.current.has(selected.node.id)
          : edgeMapRef.current.has(selected.edge.id);
      if (!exists) {
        selectionChangeRef.current(null);
      }
    }
  }, [edges, graphResetKey, mode, nodes]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) {
      return;
    }
    cy.elements().unselect();
    if (!selection) {
      return;
    }
    const element = cy.getElementById(selection.kind === "node" ? selection.node.id : selection.edge.id);
    if (element.nonempty()) {
      element.select();
    }
  }, [selection]);

  useEffect(() => {
    scheduleLabelRender();
  }, [isFullscreen]);

  function scheduleLabelRender() {
    if (!cyRef.current || !labelLayerRef.current || labelFrameRef.current !== null) {
      return;
    }
    labelFrameRef.current = window.requestAnimationFrame(() => {
      labelFrameRef.current = null;
      renderLabels();
    });
  }

  function renderLabels() {
    const cy = cyRef.current;
    const surface = surfaceRef.current;
    const layer = labelLayerRef.current;
    if (!cy || !surface || !layer) {
      return;
    }

    const width = surface.clientWidth;
    const height = surface.clientHeight;
    const viewportPadding = 140;
    const zoom = Number(cy.zoom() || 1);
    const labelScale = Math.max(0.3, Math.min(1.35, zoom));
    const labelFontPx = 11.84 * labelScale;
    const liveFontPx = 10.88 * labelScale;
    const labelMaxWidthPx = Math.max(48, Math.min(220, 150 * labelScale));
    const labelGapPx = Math.max(2, 8 * labelScale);
    const html: string[] = [];

    cy.nodes().forEach((node) => {
      const data = node.data() as VisibleGraphNode;
      const displayLabel = String(data.displayLabel || "").trim();
      const liveHoldingsLabel = String(data.live_holdings_label || "").trim();
      if (!displayLabel && !liveHoldingsLabel) {
        return;
      }

      const renderedPosition = node.renderedPosition();
      if (
        renderedPosition.x < -viewportPadding ||
        renderedPosition.x > width + viewportPadding ||
        renderedPosition.y < -viewportPadding ||
        renderedPosition.y > height + viewportPadding
      ) {
        return;
      }

      const renderedHeight = renderedNodeHeight(node);
      const topY = renderedPosition.y - renderedHeight / 2 - labelGapPx;
      const bottomY = renderedPosition.y + renderedHeight / 2 + labelGapPx;

      if (displayLabel) {
        html.push(
          `<div class="graph-node-text graph-node-label" style="left:${renderedPosition.x.toFixed(
            1
          )}px;top:${topY.toFixed(1)}px;font-size:${labelFontPx.toFixed(
            2
          )}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(displayLabel)}</div>`
        );
      }
      if (liveHoldingsLabel) {
        const unavailableClass = data.live_holdings_status === "error" ? " is-unavailable" : "";
        html.push(
          `<div class="graph-node-text graph-node-live${unavailableClass}" style="left:${renderedPosition.x.toFixed(
            1
          )}px;top:${bottomY.toFixed(1)}px;font-size:${liveFontPx.toFixed(
            2
          )}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(liveHoldingsLabel)}</div>`
        );
      }
    });

    layer.innerHTML = html.join("");
  }

  function handleToolbarAction(action: "zoom-in" | "zoom-out" | "fit" | "fullscreen" | "filters") {
    const cy = cyRef.current;
    const surface = surfaceRef.current;
    if (action === "filters") {
      filterUI?.onToggle();
      return;
    }
    if (!cy || !surface) {
      return;
    }
    const center = { x: surface.clientWidth / 2, y: surface.clientHeight / 2 };
    switch (action) {
      case "zoom-in":
        cy.zoom({ level: cy.zoom() * 1.3, renderedPosition: center });
        break;
      case "zoom-out":
        cy.zoom({ level: cy.zoom() / 1.3, renderedPosition: center });
        break;
      case "fit":
        viewportRef.current = null;
        cy.fit(cy.elements(), 40);
        scheduleLabelRender();
        break;
      case "fullscreen":
        setIsFullscreen((current) => !current);
        break;
    }
  }

  function handleContextMenuAction(action: string) {
    const menu = menuState;
    setMenuState(null);
    if (!menu) {
      return;
    }

    if (menu.mode === "pane") {
      if (action === "check-unavailable") {
        onCheckUnavailable?.();
      }
      return;
    }

    const node = menu.node;
    switch (action) {
      case "explorer":
        onOpenExplorer?.(node);
        break;
      case "copy-address":
        onCopyAddress?.(node);
        break;
      case "refresh-live-value":
        onRefreshLiveValue?.(node);
        break;
      case "expand-hop":
        onNodeDoubleActivate?.(node);
        break;
      case "label-node":
        onLabelNode?.(node);
        break;
      case "mark-asgard":
        onMarkAsgard?.(node);
        break;
      case "remove-node":
        onRemoveNode?.(node);
        break;
    }
  }

  return (
    <div className={`graph-card ${isFullscreen ? "fullscreen" : ""}`} ref={rootRef}>
      <div className="graph-container">
        <div className="graph-surface" ref={surfaceRef}>
          <div className="graph-canvas" ref={cyMountRef} />
          <div className="graph-label-layer" ref={labelLayerRef} />
          <div className="graph-selection-box" ref={selectionBoxRef} />

          <div className="graph-toolbar">
            {filterUI ? (
              <ToolbarButton
                active={filterUI.isActive || filterUI.isOpen}
                title="Filters"
                onClick={() => handleToolbarAction("filters")}
              >
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M2 3h12M4 8h8M6 13h4" />
                </svg>
              </ToolbarButton>
            ) : null}
            <ToolbarButton title="Zoom in (+)" onClick={() => handleToolbarAction("zoom-in")}>
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="8" y1="3" x2="8" y2="13" />
                <line x1="3" y1="8" x2="13" y2="8" />
              </svg>
            </ToolbarButton>
            <ToolbarButton title="Zoom out (-)" onClick={() => handleToolbarAction("zoom-out")}>
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="3" y1="8" x2="13" y2="8" />
              </svg>
            </ToolbarButton>
            <ToolbarButton title="Fit to view (0)" onClick={() => handleToolbarAction("fit")}>
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4" />
              </svg>
            </ToolbarButton>
            <ToolbarButton title="Fullscreen (F)" onClick={() => handleToolbarAction("fullscreen")}>
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M2 5V2h3M11 2h3v3M14 11v3h-3M5 14H2v-3" />
              </svg>
            </ToolbarButton>
          </div>

          {filterUI?.isOpen ? (
            <div className="graph-filter-popover" ref={filterPopoverRef}>
              {filterUI.popover}
            </div>
          ) : null}

          <div className="graph-help">
            Wheel to zoom · Left-drag to box-select · Middle-drag to pan · Right-click for actions · Double-click to{" "}
            {doubleActivateLabel.toLowerCase()}
          </div>
        </div>

        {menuState ? (
          <div
            className="graph-context-menu"
            ref={menuRef}
            style={{ left: `${menuState.x}px`, top: `${menuState.y}px` }}
          >
            {menuState.mode === "node" ? (
              <>
                {onOpenExplorer ? (
                  <ContextAction label="Open explorer" onClick={() => handleContextMenuAction("explorer")} />
                ) : null}
                {onCopyAddress ? (
                  <ContextAction label="Copy address" onClick={() => handleContextMenuAction("copy-address")} />
                ) : null}
                {onRefreshLiveValue ? (
                  <ContextAction
                    label="Refresh live value"
                    onClick={() => handleContextMenuAction("refresh-live-value")}
                  />
                ) : null}
                {onNodeDoubleActivate ? (
                  <ContextAction label={doubleActivateLabel} onClick={() => handleContextMenuAction("expand-hop")} />
                ) : null}
                {onLabelNode ? (
                  <ContextAction label="Label node" onClick={() => handleContextMenuAction("label-node")} />
                ) : null}
                {onMarkAsgard ? (
                  <ContextAction label="Mark Asgard" onClick={() => handleContextMenuAction("mark-asgard")} />
                ) : null}
                {onRemoveNode ? (
                  <ContextAction label="Remove from graph" onClick={() => handleContextMenuAction("remove-node")} />
                ) : null}
              </>
            ) : null}
            {menuState.mode === "pane" && onCheckUnavailable ? (
              <ContextAction
                label="Check unavailable live values"
                onClick={() => handleContextMenuAction("check-unavailable")}
              />
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function ToolbarButton({
  active = false,
  title,
  onClick,
  children,
}: {
  active?: boolean;
  title: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button type="button" className={active ? "is-active" : ""} title={title} onClick={onClick}>
      {children}
    </button>
  );
}

function ContextAction({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick}>
      {label}
    </button>
  );
}

function renderedNodeHeight(node: cytoscape.NodeSingular) {
  if (typeof node.renderedOuterHeight === "function") {
    return Number(node.renderedOuterHeight() || 0);
  }
  if (typeof node.renderedHeight === "function") {
    return Number(node.renderedHeight() || 0);
  }
  return 0;
}

function graphNodeAtClientPoint(
  cy: cytoscape.Core,
  surface: HTMLDivElement,
  clientX: number,
  clientY: number
) {
  const rect = surface.getBoundingClientRect();
  const x = clientX - rect.left;
  const y = clientY - rect.top;
  return cy.nodes().some((node) => {
    const box = node.renderedBoundingBox({ includeLabels: false, includeOverlays: false });
    return x >= box.x1 && x <= box.x2 && y >= box.y1 && y <= box.y2;
  });
}

async function applyElkLayout(
  cy: cytoscape.Core,
  mode: "actor" | "explorer",
  nodes: VisibleGraphNode[],
  elk: InstanceType<typeof ELK>
) {
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
    children: nodes.map((node) => {
      const size = graphLayoutNodeSize(mode, node);
      return {
        id: node.id,
        width: size.width,
        height: size.height,
      };
    }),
    edges: cy
      .edges()
      .toArray()
      .map((edge) => ({
        id: edge.id(),
        sources: [String(edge.data("source"))],
        targets: [String(edge.data("target"))],
      })),
  };

  const result = await elk.layout(graph);
  const positions = new Map(
    (result.children || []).map((child: { id: string; x?: number; y?: number }) => [
      child.id,
      { x: child.x || 0, y: child.y || 0 },
    ])
  );

  const positionMap: Record<string, { x: number; y: number }> = {};
  nodes.forEach((node) => {
    positionMap[node.id] =
      positions.get(node.id) || { x: Number(node.depth || 0) * 180, y: 80 };
  });

  cy.layout({
    name: "preset",
    fit: false,
    animate: false,
    positions: positionMap,
  }).run();
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function escapeHTML(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
