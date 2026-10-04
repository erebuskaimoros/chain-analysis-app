import { useEffect, useRef, type MutableRefObject } from "react";
import type cytoscape from "cytoscape";

const MINIMAP_PADDING = 10;
const NODE_DOT_RADIUS = 2.4;

interface MinimapProjection {
  scale: number;
  offsetX: number;
  offsetY: number;
}

// Draws a corner minimap (node dots + viewport rectangle) and lets the user
// click/drag it to pan. Redraws are rAF-throttled off cytoscape events.
export function useGraphMinimap(
  cyRef: MutableRefObject<cytoscape.Core | null>,
  canvasRef: MutableRefObject<HTMLCanvasElement | null>,
  surfaceRef: MutableRefObject<HTMLDivElement | null>
) {
  const projectionRef = useRef<MinimapProjection | null>(null);

  useEffect(() => {
    const cy = cyRef.current;
    const canvas = canvasRef.current;
    const surface = surfaceRef.current;
    if (!cy || !canvas || !surface || typeof cy.on !== "function") {
      return;
    }
    const cyInstance = cy;
    const canvasElement = canvas;
    const surfaceElement = surface;
    let frame: number | null = null;
    let dragging = false;
    let contextUnavailable = false;

    function acquireContext() {
      if (contextUnavailable) {
        return null;
      }
      let context: CanvasRenderingContext2D | null = null;
      try {
        context = typeof canvasElement.getContext === "function" ? canvasElement.getContext("2d") : null;
      } catch {
        context = null;
      }
      if (!context) {
        contextUnavailable = true;
      }
      return context;
    }

    function modelBounds() {
      const nodes = cyInstance.nodes();
      let minX = Number.POSITIVE_INFINITY;
      let minY = Number.POSITIVE_INFINITY;
      let maxX = Number.NEGATIVE_INFINITY;
      let maxY = Number.NEGATIVE_INFINITY;
      nodes.forEach((node) => {
        if (typeof node.position !== "function") {
          return;
        }
        const position = node.position();
        minX = Math.min(minX, position.x);
        minY = Math.min(minY, position.y);
        maxX = Math.max(maxX, position.x);
        maxY = Math.max(maxY, position.y);
      });
      if (!Number.isFinite(minX)) {
        return null;
      }
      // Breathing room around single nodes and tight clusters.
      const spanPad = 80;
      return {
        minX: minX - spanPad,
        minY: minY - spanPad,
        maxX: maxX + spanPad,
        maxY: maxY + spanPad,
      };
    }

    function draw() {
      const context = acquireContext();
      if (!context) {
        return;
      }
      const width = canvasElement.width;
      const height = canvasElement.height;
      context.clearRect(0, 0, width, height);

      const bounds = modelBounds();
      if (!bounds) {
        projectionRef.current = null;
        return;
      }

      const spanX = Math.max(1, bounds.maxX - bounds.minX);
      const spanY = Math.max(1, bounds.maxY - bounds.minY);
      const scale = Math.min(
        (width - MINIMAP_PADDING * 2) / spanX,
        (height - MINIMAP_PADDING * 2) / spanY
      );
      const offsetX = MINIMAP_PADDING + ((width - MINIMAP_PADDING * 2) - spanX * scale) / 2 - bounds.minX * scale;
      const offsetY = MINIMAP_PADDING + ((height - MINIMAP_PADDING * 2) - spanY * scale) / 2 - bounds.minY * scale;
      projectionRef.current = { scale, offsetX, offsetY };

      cyInstance.nodes().forEach((node) => {
        if (typeof node.position !== "function") {
          return;
        }
        const position = node.position();
        const x = position.x * scale + offsetX;
        const y = position.y * scale + offsetY;
        const color = typeof node.data === "function" ? String(node.data("color") || "#5f86be") : "#5f86be";
        context.fillStyle = color;
        context.beginPath();
        context.arc(x, y, NODE_DOT_RADIUS, 0, Math.PI * 2);
        context.fill();
      });

      // Current viewport in model coordinates → minimap rectangle.
      const zoom = Number(cyInstance.zoom() || 1);
      const pan = cyInstance.pan();
      const viewWidth = surfaceElement.clientWidth;
      const viewHeight = surfaceElement.clientHeight;
      if (zoom > 0 && viewWidth > 0 && viewHeight > 0) {
        const topLeft = { x: (0 - pan.x) / zoom, y: (0 - pan.y) / zoom };
        const bottomRight = { x: (viewWidth - pan.x) / zoom, y: (viewHeight - pan.y) / zoom };
        context.strokeStyle = "rgba(255, 221, 68, 0.85)";
        context.lineWidth = 1;
        context.strokeRect(
          topLeft.x * scale + offsetX,
          topLeft.y * scale + offsetY,
          (bottomRight.x - topLeft.x) * scale,
          (bottomRight.y - topLeft.y) * scale
        );
      }
    }

    function schedule() {
      if (frame !== null) {
        return;
      }
      frame = window.requestAnimationFrame(() => {
        frame = null;
        draw();
      });
    }

    function panToMinimapPoint(clientX: number, clientY: number) {
      const projection = projectionRef.current;
      if (!projection) {
        return;
      }
      const rect = canvasElement.getBoundingClientRect();
      const modelX = (clientX - rect.left - projection.offsetX) / projection.scale;
      const modelY = (clientY - rect.top - projection.offsetY) / projection.scale;
      const zoom = Number(cyInstance.zoom() || 1);
      cyInstance.pan({
        x: surfaceElement.clientWidth / 2 - modelX * zoom,
        y: surfaceElement.clientHeight / 2 - modelY * zoom,
      });
    }

    function onMouseDown(event: MouseEvent) {
      if (event.button !== 0) {
        return;
      }
      dragging = true;
      panToMinimapPoint(event.clientX, event.clientY);
      event.preventDefault();
      event.stopPropagation();
    }

    function onWindowMouseMove(event: MouseEvent) {
      if (!dragging) {
        return;
      }
      panToMinimapPoint(event.clientX, event.clientY);
    }

    function onWindowMouseUp() {
      dragging = false;
    }

    cyInstance.on("render pan zoom add remove position data", schedule);
    canvasElement.addEventListener("mousedown", onMouseDown);
    window.addEventListener("mousemove", onWindowMouseMove);
    window.addEventListener("mouseup", onWindowMouseUp);
    schedule();

    return () => {
      if (frame !== null) {
        window.cancelAnimationFrame(frame);
      }
      if (typeof cyInstance.off === "function") {
        cyInstance.off("render pan zoom add remove position data", schedule);
      }
      canvasElement.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("mousemove", onWindowMouseMove);
      window.removeEventListener("mouseup", onWindowMouseUp);
    };
  }, [canvasRef, cyRef, surfaceRef]);
}
