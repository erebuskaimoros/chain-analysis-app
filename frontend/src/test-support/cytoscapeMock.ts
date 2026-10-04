import { vi } from "vitest";
import type { VisibleGraphNode } from "../lib/graph";

export type MockElementData = VisibleGraphNode & {
  source?: string;
  target?: string;
};

type Handler = (...args: unknown[]) => void;

function splitClasses(names: string) {
  return String(names || "")
    .split(/\s+/)
    .filter(Boolean);
}

export class MockNodeElement {
  readonly core: MockCyCore;
  private readonly dataValue: MockElementData;
  private selected = false;
  private positionValue: { x: number; y: number };
  readonly classes = new Set<string>();
  removed = false;

  constructor(core: MockCyCore, dataValue: MockElementData, index: number) {
    this.core = core;
    this.dataValue = dataValue;
    this.positionValue = { x: 40 + index * 80, y: 40 };
  }

  isNode() {
    return true;
  }

  isEdge() {
    return false;
  }

  id() {
    return this.dataValue.id;
  }

  data(keyOrData?: string | Record<string, unknown>) {
    if (typeof keyOrData === "string") {
      return (this.dataValue as unknown as Record<string, unknown>)[keyOrData];
    }
    if (keyOrData && typeof keyOrData === "object") {
      Object.assign(this.dataValue as unknown as Record<string, unknown>, keyOrData);
      return this;
    }
    return this.dataValue;
  }

  renderedBoundingBox() {
    const left = this.positionValue.x - 20;
    return {
      x1: left,
      y1: this.positionValue.y - 20,
      x2: left + 40,
      y2: this.positionValue.y + 20,
    };
  }

  renderedPosition() {
    return { ...this.positionValue };
  }

  renderedOuterHeight() {
    return 40;
  }

  renderedHeight() {
    return 40;
  }

  visible() {
    return true;
  }

  select() {
    this.selected = true;
    return this;
  }

  unselect() {
    this.selected = false;
    return this;
  }

  isSelected() {
    return this.selected;
  }

  position(next?: { x: number; y: number }) {
    if (next) {
      this.positionValue = { ...next };
    }
    return { ...this.positionValue };
  }

  nonempty() {
    return true;
  }

  empty() {
    return false;
  }

  remove() {
    this.removed = true;
    return this;
  }

  addClass(names: string) {
    splitClasses(names).forEach((name) => this.classes.add(name));
    return this;
  }

  removeClass(names: string) {
    splitClasses(names).forEach((name) => this.classes.delete(name));
    return this;
  }

  hasClass(name: string) {
    return this.classes.has(name);
  }

  union(other: MockCollection | MockNodeElement | MockEdgeElement) {
    return makeCollection(this.core, dedupeElements([this, ...collectionItems(other)]));
  }

  closedNeighborhood() {
    const id = this.id();
    const incident = this.core
      .allEdges()
      .filter((edge) => edge.data("source") === id || edge.data("target") === id);
    const neighborIDs = new Set<string>();
    incident.forEach((edge) => {
      neighborIDs.add(String(edge.data("source")));
      neighborIDs.add(String(edge.data("target")));
    });
    const neighbors = this.core.allNodes().filter((node) => neighborIDs.has(node.id()));
    return makeCollection(this.core, dedupeElements([this, ...incident, ...neighbors]));
  }
}

export class MockEdgeElement {
  readonly core: MockCyCore;
  private readonly dataValue: MockElementData;
  private selected = false;
  readonly classes = new Set<string>();
  removed = false;

  constructor(core: MockCyCore, dataValue: MockElementData) {
    this.core = core;
    this.dataValue = dataValue;
  }

  isNode() {
    return false;
  }

  isEdge() {
    return true;
  }

  id() {
    return this.dataValue.id;
  }

  data(keyOrData?: string | Record<string, unknown>) {
    if (typeof keyOrData === "string") {
      return (this.dataValue as unknown as Record<string, unknown>)[keyOrData];
    }
    if (keyOrData && typeof keyOrData === "object") {
      Object.assign(this.dataValue as unknown as Record<string, unknown>, keyOrData);
      return this;
    }
    return this.dataValue;
  }

  nonempty() {
    return true;
  }

  empty() {
    return false;
  }

  select() {
    this.selected = true;
    return this;
  }

  unselect() {
    this.selected = false;
    return this;
  }

  isSelected() {
    return this.selected;
  }

  remove() {
    this.removed = true;
    return this;
  }

  addClass(names: string) {
    splitClasses(names).forEach((name) => this.classes.add(name));
    return this;
  }

  removeClass(names: string) {
    splitClasses(names).forEach((name) => this.classes.delete(name));
    return this;
  }

  hasClass(name: string) {
    return this.classes.has(name);
  }

  union(other: MockCollection | MockNodeElement | MockEdgeElement) {
    return makeCollection(this.core, dedupeElements([this, ...collectionItems(other)]));
  }

  connectedNodes() {
    const source = String(this.data("source"));
    const target = String(this.data("target"));
    return makeCollection(
      this.core,
      this.core.allNodes().filter((node) => node.id() === source || node.id() === target)
    );
  }

  closedNeighborhood() {
    return makeCollection(this.core, dedupeElements([this, ...collectionItems(this.connectedNodes())]));
  }
}

export class EmptyElement {
  nonempty() {
    return false;
  }

  empty() {
    return true;
  }

  select() {
    return this;
  }

  unselect() {
    return this;
  }
}

type MockElement = MockNodeElement | MockEdgeElement;

export type MockCollection = MockElement[] & {
  toArray: () => MockElement[];
  remove: () => void;
  select: () => void;
  unselect: () => void;
  addClass: (names: string) => MockCollection;
  removeClass: (names: string) => MockCollection;
  union: (other: MockCollection | MockElement) => MockCollection;
  closedNeighborhood: () => MockCollection;
  empty: () => boolean;
  nonempty: () => boolean;
};

function collectionItems(value: MockCollection | MockElement): MockElement[] {
  return Array.isArray(value) ? [...value] : [value];
}

function dedupeElements(items: MockElement[]) {
  const seen = new Set<MockElement>();
  const out: MockElement[] = [];
  items.forEach((item) => {
    if (!seen.has(item)) {
      seen.add(item);
      out.push(item);
    }
  });
  return out;
}

export function makeCollection(core: MockCyCore, items: MockElement[]): MockCollection {
  const collection = [...items] as MockCollection;
  collection.toArray = () => [...items];
  collection.remove = () => {
    items.forEach((item) => item.remove());
  };
  collection.select = () => {
    items.forEach((item) => item.select());
  };
  collection.unselect = () => {
    items.forEach((item) => item.unselect());
  };
  collection.addClass = (names: string) => {
    items.forEach((item) => item.addClass(names));
    return collection;
  };
  collection.removeClass = (names: string) => {
    items.forEach((item) => item.removeClass(names));
    return collection;
  };
  collection.union = (other) => makeCollection(core, dedupeElements([...items, ...collectionItems(other)]));
  collection.closedNeighborhood = () =>
    makeCollection(
      core,
      dedupeElements(
        items.flatMap((item) =>
          typeof item.closedNeighborhood === "function" ? collectionItems(item.closedNeighborhood()) : [item]
        )
      )
    );
  collection.empty = () => items.length === 0;
  collection.nonempty = () => items.length > 0;
  return collection;
}

export class MockCyCore {
  private nodeElements: MockNodeElement[] = [];
  private edgeElements: MockEdgeElement[] = [];
  private readonly handlers = new Map<string, Handler[]>();
  private zoomLevel = 1;
  private panValue = { x: 0, y: 0 };
  private addIndex = 0;
  lastCenteredID: string | null = null;

  allNodes() {
    return this.nodeElements.filter((node) => !node.removed);
  }

  allEdges() {
    return this.edgeElements.filter((edge) => !edge.removed);
  }

  add(elements: Array<{ data: MockElementData }>) {
    for (const element of elements) {
      if (typeof element.data.source === "string" && typeof element.data.target === "string") {
        this.edgeElements.push(new MockEdgeElement(this, element.data));
        continue;
      }
      this.nodeElements.push(new MockNodeElement(this, element.data, this.addIndex));
      this.addIndex += 1;
    }
  }

  elements(selector?: string) {
    const live = [...this.allNodes(), ...this.allEdges()];
    const items =
      selector === ":selected" ? live.filter((element) => element.isSelected()) : live;
    return makeCollection(this, items);
  }

  nodes(selector?: string) {
    const live = this.allNodes();
    const items = selector === ":selected" ? live.filter((node) => node.isSelected()) : live;
    return makeCollection(this, items);
  }

  edges() {
    return makeCollection(this, this.allEdges());
  }

  getElementById(id: string) {
    return (
      this.allNodes().find((node) => node.id() === id) ??
      this.allEdges().find((edge) => edge.id() === id) ??
      new EmptyElement()
    );
  }

  batch(callback: () => void) {
    callback();
  }

  on(eventName: string, selectorOrHandler: unknown, maybeHandler?: unknown) {
    const handler =
      typeof selectorOrHandler === "function"
        ? (selectorOrHandler as Handler)
        : (maybeHandler as Handler);
    const current = this.handlers.get(eventName) ?? [];
    current.push(handler);
    this.handlers.set(eventName, current);
  }

  off() {}

  emit(eventName: string, event: Record<string, unknown>) {
    const handlers = this.handlers.get(eventName) ?? [];
    handlers.forEach((handler) => handler(event));
  }

  layout(options?: { positions?: Record<string, { x: number; y: number }> }) {
    return {
      run: () => {
        const positions = options?.positions ?? {};
        for (const node of this.allNodes()) {
          const nextPosition = positions[node.id()];
          if (nextPosition) {
            node.position(nextPosition);
          }
        }
      },
    };
  }

  fit() {}

  resize() {}

  animate(options?: { center?: { eles?: { id?: () => string } } }) {
    const target = options?.center?.eles;
    if (target && typeof target.id === "function") {
      this.lastCenteredID = target.id();
    }
  }

  zoom(value?: number | { level: number; renderedPosition?: { x: number; y: number } }) {
    if (typeof value === "number") {
      this.zoomLevel = value;
    } else if (value && typeof value.level === "number") {
      this.zoomLevel = value.level;
    }
    return this.zoomLevel;
  }

  pan(value?: { x: number; y: number }) {
    if (value) {
      this.panValue = { ...value };
    }
    return this.panValue;
  }

  minZoom() {
    return 0.05;
  }

  maxZoom() {
    return 10;
  }

  destroy() {}

  setSelectedNodeIDs(ids: string[]) {
    const selected = new Set(ids);
    this.allNodes().forEach((node) => {
      if (selected.has(node.id())) {
        node.select();
      } else {
        node.unselect();
      }
    });
  }

  nodePosition(id: string) {
    return this.allNodes().find((node) => node.id() === id)?.position() ?? null;
  }

  setNodePosition(id: string, position: { x: number; y: number }) {
    this.allNodes().find((node) => node.id() === id)?.position(position);
  }
}

export const mockCytoscapeState: {
  latestCore: MockCyCore | null;
  lastOptions: Record<string, unknown> | null;
} = {
  latestCore: null,
  lastOptions: null,
};

export const mockCytoscapeFactory = vi.fn((options?: Record<string, unknown>) => {
  mockCytoscapeState.lastOptions = options ?? null;
  const core = new MockCyCore();
  mockCytoscapeState.latestCore = core;
  return core;
});

export class MockELK {
  async layout(graph: { children?: Array<{ id: string }> }) {
    return {
      children: (graph.children ?? []).map((child, index) => ({
        id: child.id,
        x: 40 + index * 80,
        y: 40,
      })),
    };
  }
}
