import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GraphCanvas } from "../../GraphCanvas";
import { makeVisibleNode } from "../../../../test-support/graphFixtures";
import type { VisibleGraphNode } from "../../../../lib/graph";

type MockElementData = VisibleGraphNode & {
  source?: string;
  target?: string;
};

const cytoscapeState = vi.hoisted(() => ({
  latestCore: null as MockCyCore | null,
}));

class MockNodeElement {
  private readonly dataValue: MockElementData;
  private selected = false;
  private readonly index: number;

  constructor(dataValue: MockElementData, index: number) {
    this.dataValue = dataValue;
    this.index = index;
  }

  id() {
    return this.dataValue.id;
  }

  data(key?: string) {
    if (typeof key === "string") {
      return (this.dataValue as unknown as Record<string, unknown>)[key];
    }
    return this.dataValue;
  }

  renderedBoundingBox() {
    const left = 20 + this.index * 80;
    return {
      x1: left,
      y1: 20,
      x2: left + 40,
      y2: 60,
    };
  }

  renderedPosition() {
    const box = this.renderedBoundingBox();
    return {
      x: (box.x1 + box.x2) / 2,
      y: (box.y1 + box.y2) / 2,
    };
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

  nonempty() {
    return true;
  }
}

class MockEdgeElement {
  private readonly dataValue: MockElementData;

  constructor(dataValue: MockElementData) {
    this.dataValue = dataValue;
  }

  id() {
    return this.dataValue.id;
  }

  data(key?: string) {
    if (typeof key === "string") {
      return (this.dataValue as unknown as Record<string, unknown>)[key];
    }
    return this.dataValue;
  }

  nonempty() {
    return true;
  }

  select() {
    return this;
  }

  unselect() {
    return this;
  }
}

class EmptyElement {
  nonempty() {
    return false;
  }

  select() {
    return this;
  }
}

function createCollection<T extends { unselect?: () => unknown }>(items: T[], removeAll?: () => void) {
  return Object.assign([...items], {
    toArray: () => [...items],
    remove: () => {
      removeAll?.();
    },
    unselect: () => {
      items.forEach((item) => item.unselect?.());
    },
  });
}

class MockCyCore {
  private nodeElements: MockNodeElement[] = [];
  private edgeElements: MockEdgeElement[] = [];
  private readonly handlers = new Map<string, Array<(...args: unknown[]) => void>>();
  private zoomLevel = 1;
  private panValue = { x: 0, y: 0 };

  add(elements: Array<{ data: MockElementData }>) {
    this.nodeElements = [];
    this.edgeElements = [];
    let nodeIndex = 0;
    for (const element of elements) {
      if (typeof element.data.source === "string" && typeof element.data.target === "string") {
        this.edgeElements.push(new MockEdgeElement(element.data));
        continue;
      }
      this.nodeElements.push(new MockNodeElement(element.data, nodeIndex));
      nodeIndex += 1;
    }
  }

  elements(selector?: string) {
    const items =
      selector === ":selected"
        ? this.nodeElements.filter((node) => node.isSelected())
        : [...this.nodeElements, ...this.edgeElements];
    return createCollection(items, () => {
      this.nodeElements = [];
      this.edgeElements = [];
    });
  }

  nodes(selector?: string) {
    const items = selector === ":selected" ? this.nodeElements.filter((node) => node.isSelected()) : this.nodeElements;
    return createCollection(items);
  }

  edges() {
    return createCollection(this.edgeElements);
  }

  getElementById(id: string) {
    return this.nodeElements.find((node) => node.id() === id) ?? this.edgeElements.find((edge) => edge.id() === id) ?? new EmptyElement();
  }

  batch(callback: () => void) {
    callback();
  }

  on(eventName: string, selectorOrHandler: unknown, maybeHandler?: unknown) {
    const handler =
      typeof selectorOrHandler === "function"
        ? (selectorOrHandler as (...args: unknown[]) => void)
        : (maybeHandler as (...args: unknown[]) => void);
    const current = this.handlers.get(eventName) ?? [];
    current.push(handler);
    this.handlers.set(eventName, current);
  }

  layout() {
    return {
      run: () => undefined,
    };
  }

  fit() {}

  resize() {}

  zoom(value?: number) {
    if (typeof value === "number") {
      this.zoomLevel = value;
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
    this.nodeElements.forEach((node) => {
      if (selected.has(node.id())) {
        node.select();
      } else {
        node.unselect();
      }
    });
  }
}

vi.mock("cytoscape", () => ({
  default: vi.fn(() => {
    const core = new MockCyCore();
    cytoscapeState.latestCore = core;
    return core;
  }),
}));

vi.mock("elkjs/lib/elk.bundled.js", () => ({
  default: class MockELK {
    async layout(graph: { children?: Array<{ id: string }> }) {
      return {
        children: (graph.children ?? []).map((child, index) => ({
          id: child.id,
          x: index * 120,
          y: 80,
        })),
      };
    }
  },
}));

describe("GraphCanvas multi-node context menu", () => {
  beforeEach(() => {
    cytoscapeState.latestCore = null;
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("shows the multi-node context menu when multiple cytoscape nodes are selected", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const onSelectionChange = vi.fn();

    const { container } = render(
      <GraphCanvas
        mode="explorer"
        nodes={[nodeA, nodeB]}
        edges={[]}
        selection={{ kind: "node", node: nodeA }}
        onSelectionChange={onSelectionChange}
      />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface) {
      return;
    }

    Object.defineProperty(surface, "clientWidth", { configurable: true, value: 640 });
    Object.defineProperty(surface, "clientHeight", { configurable: true, value: 480 });
    surface.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        right: 640,
        bottom: 480,
        width: 640,
        height: 480,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect;

    expect(cytoscapeState.latestCore).not.toBeNull();
    cytoscapeState.latestCore?.setSelectedNodeIDs(["node-a", "node-b"]);

    fireEvent.contextMenu(surface, { clientX: 300, clientY: 300 });

    const menuAction = await screen.findByRole("button", { name: "Expand Nodes (2)" });
    expect(menuAction).toBeTruthy();
  });

  it("shows the multi-node context menu when right-clicking directly on one of the selected nodes", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    const { container } = render(
      <GraphCanvas
        mode="explorer"
        nodes={[nodeA, nodeB]}
        edges={[]}
        selection={{ kind: "nodes", nodes: [nodeA, nodeB] }}
        onSelectionChange={vi.fn()}
      />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface) {
      return;
    }

    Object.defineProperty(surface, "clientWidth", { configurable: true, value: 640 });
    Object.defineProperty(surface, "clientHeight", { configurable: true, value: 480 });
    surface.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        right: 640,
        bottom: 480,
        width: 640,
        height: 480,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect;

    cytoscapeState.latestCore?.setSelectedNodeIDs(["node-a", "node-b"]);

    fireEvent.contextMenu(surface, { clientX: 40, clientY: 40 });

    const menuAction = await screen.findByRole("button", { name: "Expand Nodes (2)" });
    expect(menuAction).toBeTruthy();
  });
});
