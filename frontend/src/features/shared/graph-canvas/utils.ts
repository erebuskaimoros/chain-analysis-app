import cytoscape from "cytoscape";

export function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

export function renderedNodeHeight(node: cytoscape.NodeSingular) {
  if (typeof node.renderedOuterHeight === "function") {
    return Number(node.renderedOuterHeight() || 0);
  }
  if (typeof node.renderedHeight === "function") {
    return Number(node.renderedHeight() || 0);
  }
  return 0;
}

export function graphNodeAtClientPoint(
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

export function contextMenuPointFromGraphEvent(surface: HTMLDivElement, event: cytoscape.EventObject) {
  const pointerPoint = clientPointFromEvent(event.originalEvent);
  if (pointerPoint) {
    return pointerPoint;
  }

  const rect = surface.getBoundingClientRect();
  const target = event.target as { renderedPosition?: () => cytoscape.Position } | undefined;
  const renderedPosition = event.renderedPosition ||
    target?.renderedPosition?.() || {
      x: surface.clientWidth / 2,
      y: surface.clientHeight / 2,
    };

  return {
    x: rect.left + renderedPosition.x,
    y: rect.top + renderedPosition.y,
  };
}

export function escapeHTML(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function clientPointFromEvent(event: Event | null | undefined) {
  if (!event) {
    return null;
  }
  if (event instanceof MouseEvent) {
    return { x: event.clientX, y: event.clientY };
  }
  if (typeof TouchEvent !== "undefined" && event instanceof TouchEvent) {
    const touch = event.changedTouches[0] || event.touches[0];
    if (touch) {
      return { x: touch.clientX, y: touch.clientY };
    }
  }
  return null;
}
