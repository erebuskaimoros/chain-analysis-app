import cytoscape from "cytoscape";
import type { VisibleGraphNode } from "../../../lib/graph";

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
): VisibleGraphNode | null {
  const rect = surface.getBoundingClientRect();
  const x = clientX - rect.left;
  const y = clientY - rect.top;
  for (const node of cy.nodes().toArray()) {
    const box = node.renderedBoundingBox({ includeLabels: false, includeOverlays: false });
    if (x >= box.x1 && x <= box.x2 && y >= box.y1 && y <= box.y2) {
      return node.data() as VisibleGraphNode;
    }
  }
  return null;
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

export function selectedGraphNodes(cy: cytoscape.Core): VisibleGraphNode[] {
  return cy
    .nodes(":selected")
    .map((node) => node.data() as VisibleGraphNode)
    .filter(Boolean);
}

export function clusterGraphNodes(cy: cytoscape.Core, nodeIDs: string[]) {
  const elements = nodeIDs
    .map((id) => cy.getElementById(id) as cytoscape.NodeSingular)
    .filter((element) => typeof element?.position === "function");
  if (elements.length < 2) {
    return false;
  }

  const positions = elements.map((node) => ({
    node,
    position: node.position(),
    distance: renderedNodeDiameter(node),
  }));

  const center = {
    x: (Math.min(...positions.map((item) => item.position.x)) + Math.max(...positions.map((item) => item.position.x))) / 2,
    y: (Math.min(...positions.map((item) => item.position.y)) + Math.max(...positions.map((item) => item.position.y))) / 2,
  };
  const spacing = Math.max(56, average(positions.map((item) => item.distance)) * 1.45);
  const offsets = buildClusterOffsets(positions.length, spacing);
  const sortedNodes = [...positions].sort((left, right) => {
    const angleDelta = angleFrom(center, left.position) - angleFrom(center, right.position);
    if (Math.abs(angleDelta) > 0.0001) {
      return angleDelta;
    }
    return distanceFrom(center, left.position) - distanceFrom(center, right.position);
  });

  cy.batch(() => {
    sortedNodes.forEach((item, index) => {
      const offset = offsets[index] ?? { x: 0, y: 0 };
      item.node.position({
        x: center.x + offset.x,
        y: center.y + offset.y,
      });
    });
  });
  return true;
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

function renderedNodeDiameter(node: cytoscape.NodeSingular) {
  const box = node.renderedBoundingBox({ includeLabels: false, includeOverlays: false });
  const width = Math.max(0, Number(box.x2) - Number(box.x1));
  const height = Math.max(renderedNodeHeight(node), Math.max(0, Number(box.y2) - Number(box.y1)));
  return Math.max(40, width, height);
}

function buildClusterOffsets(count: number, radiusStep: number) {
  if (count <= 0) {
    return [];
  }
  const offsets = [{ x: 0, y: 0 }];
  let placed = 1;
  let ring = 1;

  while (placed < count) {
    const circumference = 2 * Math.PI * ring * radiusStep;
    const ringSlots = Math.max(6 * ring, Math.round(circumference / radiusStep));
    const pointsThisRing = Math.min(count - placed, ringSlots);
    for (let index = 0; index < pointsThisRing; index += 1) {
      const angle = (index / pointsThisRing) * Math.PI * 2;
      offsets.push({
        x: Math.cos(angle) * ring * radiusStep,
        y: Math.sin(angle) * ring * radiusStep,
      });
    }
    placed += pointsThisRing;
    ring += 1;
  }

  const centroid = {
    x: average(offsets.map((offset) => offset.x)),
    y: average(offsets.map((offset) => offset.y)),
  };

  return offsets.map((offset) => ({
    x: offset.x - centroid.x,
    y: offset.y - centroid.y,
  }));
}

function average(values: number[]) {
  if (!values.length) {
    return 0;
  }
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function angleFrom(origin: { x: number; y: number }, point: { x: number; y: number }) {
  return Math.atan2(point.y - origin.y, point.x - origin.x);
}

function distanceFrom(origin: { x: number; y: number }, point: { x: number; y: number }) {
  return Math.hypot(point.x - origin.x, point.y - origin.y);
}
