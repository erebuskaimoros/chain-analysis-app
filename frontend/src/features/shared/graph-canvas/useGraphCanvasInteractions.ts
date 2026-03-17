import { useEffect, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import type cytoscape from "cytoscape";
import type { SavedGraphCanvasState } from "../../../lib/graphState";
import { clusterGraphNodes, graphNodeAtClientPoint, clamp, selectedGraphNodes } from "./utils";
import type { ContextMenuState, GraphCanvasFilters, GraphCanvasNodeMenuActions, GraphCanvasPaneMenuActions } from "./types";
import type { GraphSelection, VisibleGraphNode } from "../../../lib/graph";

const TRACKPAD_PAN_GESTURE_LOCK_MS = 400;
const TRACKPAD_PAN_PIXEL_DELTA_THRESHOLD = 100;
const WHEEL_ZOOM_SENSITIVITY = 0.0015;
const PINCH_ZOOM_SENSITIVITY = 0.0035;

interface UseGraphCanvasInteractionsOptions {
  cyRef: MutableRefObject<cytoscape.Core | null>;
  viewportRef: MutableRefObject<{ zoom: number; pan: cytoscape.Position } | null>;
  suppressTapUntilRef: MutableRefObject<number>;
  rootRef: MutableRefObject<HTMLDivElement | null>;
  surfaceRef: MutableRefObject<HTMLDivElement | null>;
  selectionBoxRef: MutableRefObject<HTMLDivElement | null>;
  filterPopoverRef: MutableRefObject<HTMLDivElement | null>;
  menuRef: MutableRefObject<HTMLDivElement | null>;
  selection: GraphSelection;
  onSelectionChange: (selection: GraphSelection) => void;
  filters?: GraphCanvasFilters;
  nodeMenuActions?: GraphCanvasNodeMenuActions;
  paneMenuActions?: GraphCanvasPaneMenuActions;
  menuState: ContextMenuState;
  setMenuState: Dispatch<SetStateAction<ContextMenuState>>;
  isFullscreen: boolean;
  setIsFullscreen: Dispatch<SetStateAction<boolean>>;
  onNodeDoubleActivate?: (node: VisibleGraphNode) => void;
  onSaveState?: (canvasState: SavedGraphCanvasState) => void;
  scheduleLabelRender: () => void;
}

export function useGraphCanvasInteractions({
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
}: UseGraphCanvasInteractionsOptions) {
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
    let lastTrackpadPanAt = 0;

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

    function syncSelectionFromSelectedNodes() {
      const selectedNodes = cyInstance
        .nodes(":selected")
        .map((node) => node.data() as VisibleGraphNode)
        .filter(Boolean);
      if (!selectedNodes.length) {
        onSelectionChange(null);
        return;
      }
      if (selectedNodes.length === 1) {
        onSelectionChange({ kind: "node", node: selectedNodes[0] });
        return;
      }
      onSelectionChange({ kind: "nodes", nodes: selectedNodes });
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
        syncSelectionFromSelectedNodes();
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
      if (shouldPanFromWheel(event, lastTrackpadPanAt)) {
        lastTrackpadPanAt = Date.now();
        const currentPan = cyInstance.pan();
        const panScale = wheelPanScale(event, surfaceElement);
        cyInstance.pan({
          x: currentPan.x - event.deltaX * panScale,
          y: currentPan.y - event.deltaY * panScale,
        });
        event.preventDefault();
        return;
      }

      const rect = surfaceElement.getBoundingClientRect();
      const renderedPosition = {
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      };
      const currentZoom = Number(cyInstance.zoom() || 1);
      const zoomSensitivity = event.ctrlKey ? PINCH_ZOOM_SENSITIVITY : WHEEL_ZOOM_SENSITIVITY;
      const zoomFactor = Math.exp(-event.deltaY * zoomSensitivity);
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
      const hitNode = hitNodeAtClientPoint(event.clientX, event.clientY);
      const selectedNodes = selectedGraphNodes(cyInstance);
      if (hitNode) {
        if (selectedNodes.length > 1 && selectedNodes.some((n) => n.id === hitNode.id)) {
          setMenuState({ mode: "nodes", nodes: selectedNodes, x: event.clientX, y: event.clientY });
          if (selection?.kind !== "nodes") {
            onSelectionChange({ kind: "nodes", nodes: selectedNodes });
          }
          return;
        }
        setMenuState({ mode: "node", node: hitNode, x: event.clientX, y: event.clientY });
        return;
      }
      if (selectedNodes.length > 1) {
        setMenuState({ mode: "nodes", nodes: selectedNodes, x: event.clientX, y: event.clientY });
        if (selection?.kind !== "nodes") {
          onSelectionChange({ kind: "nodes", nodes: selectedNodes });
        }
        return;
      }
      setMenuState({ mode: "pane", x: event.clientX, y: event.clientY });
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
  }, [cyRef, onSelectionChange, scheduleLabelRender, selection, selectionBoxRef, setMenuState, surfaceRef, suppressTapUntilRef]);

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
        filters?.onClose();
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
        filters?.isOpen &&
        target &&
        filterPopoverRef.current &&
        !filterPopoverRef.current.contains(target) &&
        !targetElement?.closest(".graph-toolbar")
      ) {
        filters.onClose();
      }
    }

    window.addEventListener("keydown", onKeyDown);
    document.addEventListener("mousedown", onDocumentMouseDown);

    return () => {
      window.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("mousedown", onDocumentMouseDown);
    };
  }, [cyRef, filterPopoverRef, filters, menuRef, scheduleLabelRender, setIsFullscreen, setMenuState, surfaceRef, viewportRef]);

  useEffect(() => {
    const shell = rootRef.current?.closest(".graph-card-shell");
    if (shell instanceof HTMLElement) {
      shell.classList.toggle("fullscreen", isFullscreen);
    }
    document.body.style.overflow = isFullscreen ? "hidden" : "";
    const cy = cyRef.current;
    if (!cy) {
      return () => {
        if (shell instanceof HTMLElement) {
          shell.classList.remove("fullscreen");
        }
        document.body.style.overflow = "";
      };
    }

    const frame = window.requestAnimationFrame(() => {
      cy.resize();
      if (!isFullscreen) {
        cy.fit(cy.elements(), 40);
        viewportRef.current = null;
      }
      scheduleLabelRender();
    });

    return () => {
      window.cancelAnimationFrame(frame);
      if (shell instanceof HTMLElement) {
        shell.classList.remove("fullscreen");
      }
      document.body.style.overflow = "";
    };
  }, [cyRef, isFullscreen, rootRef, scheduleLabelRender, viewportRef]);

  useEffect(() => {
    const menu = menuState;
    const menuElement = menuRef.current;
    if (!menu || !menuElement) {
      return;
    }
    const menuRect = menuElement.getBoundingClientRect();
    const clampedX = clamp(menu.x, 8, Math.max(8, window.innerWidth - menuRect.width - 8));
    const clampedY = clamp(menu.y, 8, Math.max(8, window.innerHeight - menuRect.height - 8));
    if (clampedX === menu.x && clampedY === menu.y) {
      return;
    }
    setMenuState((current) => {
      if (!current || (current.x === clampedX && current.y === clampedY)) {
        return current;
      }
      return { ...current, x: clampedX, y: clampedY };
    });
  }, [menuRef, menuState, setMenuState]);

  function captureCanvasState() {
    const cy = cyRef.current;
    if (!cy) {
      return null;
    }

    const nodePositions: SavedGraphCanvasState["node_positions"] = {};
    cy.nodes().forEach((node) => {
      if (typeof node.position !== "function") {
        return;
      }
      const position = node.position();
      if (!Number.isFinite(position.x) || !Number.isFinite(position.y)) {
        return;
      }
      nodePositions[node.id()] = {
        x: position.x,
        y: position.y,
      };
    });

    const pan = cy.pan();
    const zoom = cy.zoom();
    return {
      node_positions: nodePositions,
      viewport:
        Number.isFinite(zoom) && Number.isFinite(pan.x) && Number.isFinite(pan.y)
          ? {
              zoom,
              pan: {
                x: pan.x,
                y: pan.y,
              },
            }
          : undefined,
    } satisfies SavedGraphCanvasState;
  }

  function handleToolbarAction(action: "zoom-in" | "zoom-out" | "fit" | "fullscreen" | "filters" | "save") {
    const cy = cyRef.current;
    const surface = surfaceRef.current;
    if (action === "filters") {
      filters?.onToggle();
      return;
    }
    if (action === "save") {
      const canvasState = captureCanvasState();
      if (canvasState) {
        onSaveState?.(canvasState);
      }
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
        paneMenuActions?.onCheckUnavailable?.();
      }
      return;
    }

    if (menu.mode === "nodes") {
      if (action === "expand-nodes") {
        nodeMenuActions?.onExpandNodes?.(menu.nodes);
      } else if (action === "cluster-nodes") {
        const cy = cyRef.current;
        if (cy && clusterGraphNodes(cy, menu.nodes.map((node) => node.id))) {
          scheduleLabelRender();
        }
      }
      return;
    }

    const node = menu.node;
    switch (action) {
      case "explorer":
        nodeMenuActions?.onOpenExplorer?.(node);
        break;
      case "copy-address":
        nodeMenuActions?.onCopyAddress?.(node);
        break;
      case "refresh-live-value":
        nodeMenuActions?.onRefreshLiveValue?.(node);
        break;
      case "expand-hop":
        onNodeDoubleActivate?.(node);
        break;
      case "label-node":
        nodeMenuActions?.onLabelNode?.(node);
        break;
      case "mark-asgard":
        nodeMenuActions?.onMarkAsgard?.(node);
        break;
      case "remove-node":
        nodeMenuActions?.onRemoveNode?.(node);
        break;
    }
  }

  return {
    handleToolbarAction,
    handleContextMenuAction,
  };
}

/**
 * In Chrome/Edge, WheelEvent carries a non-standard `wheelDelta` property.
 * Physical mouse scroll wheels always produce values that are exact multiples
 * of 120; trackpad gestures do not.  Returns true only when we can positively
 * identify a mouse wheel — returning false means "inconclusive", not "trackpad".
 */
function isMouseWheelByWheelDelta(event: WheelEvent): boolean {
  const wheelDelta = (event as unknown as { wheelDelta?: number }).wheelDelta;
  if (typeof wheelDelta !== "number" || wheelDelta === 0) {
    return false;
  }
  return wheelDelta % 120 === 0;
}

function shouldPanFromWheel(event: WheelEvent, lastTrackpadPanAt: number) {
  // Ctrl+wheel or trackpad pinch-to-zoom → always zoom
  if (event.ctrlKey) {
    return false;
  }

  // Continue panning if we recently detected trackpad input (covers inertia scrolling)
  const now = Date.now();
  if (now - lastTrackpadPanAt <= TRACKPAD_PAN_GESTURE_LOCK_MS) {
    return true;
  }

  // Non-pixel delta modes (LINE, PAGE) are always discrete mouse wheels
  const pixelDeltaMode = event.deltaMode === WheelEvent.DOM_DELTA_PIXEL || event.deltaMode === 0;
  if (!pixelDeltaMode) {
    return false;
  }

  // Chrome/Edge: positively identify mouse wheel via wheelDelta multiple-of-120
  if (isMouseWheelByWheelDelta(event)) {
    return false;
  }

  // Horizontal scroll component → trackpad (mice don't produce horizontal wheel events)
  const absDeltaX = Math.abs(event.deltaX);
  if (absDeltaX >= 0.5) {
    return true;
  }

  // Fractional deltas → trackpad (mouse wheels produce integer values)
  if (!Number.isInteger(event.deltaX) || !Number.isInteger(event.deltaY)) {
    return true;
  }

  // Fallback for Firefox/Safari: vertical-only, integer, pixel-mode, no wheelDelta info.
  // Mouse wheel notches on macOS typically produce ≥100px; trackpad scrolls are smaller.
  const absDeltaY = Math.abs(event.deltaY);
  return absDeltaY > 0 && absDeltaY < TRACKPAD_PAN_PIXEL_DELTA_THRESHOLD;
}

function wheelPanScale(event: WheelEvent, surfaceElement: HTMLDivElement) {
  switch (event.deltaMode) {
    case WheelEvent.DOM_DELTA_LINE:
      return 16;
    case WheelEvent.DOM_DELTA_PAGE:
      return Math.max(surfaceElement.clientWidth, surfaceElement.clientHeight);
    default:
      return 1;
  }
}
