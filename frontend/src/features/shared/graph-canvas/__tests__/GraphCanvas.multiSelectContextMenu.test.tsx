import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
  private positionValue: { x: number; y: number };

  constructor(dataValue: MockElementData, index: number) {
    this.dataValue = dataValue;
    this.positionValue = { x: 40 + index * 80, y: 40 };
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

  layout(options?: { positions?: Record<string, { x: number; y: number }> }) {
    return {
      run: () => {
        const positions = options?.positions ?? {};
        for (const node of this.nodeElements) {
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

  nodePosition(id: string) {
    return this.nodeElements.find((node) => node.id() === id)?.position() ?? null;
  }

  setNodePosition(id: string, position: { x: number; y: number }) {
    this.nodeElements.find((node) => node.id() === id)?.position(position);
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
          x: 40 + index * 80,
          y: 40,
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
    expect(screen.getByRole("button", { name: "Cluster Nodes" })).toBeTruthy();
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

  it("clusters the selected nodes around the middle of their current positions", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const nodeC = makeVisibleNode({ id: "node-c", label: "Node C" });

    const { container } = render(
      <GraphCanvas
        mode="explorer"
        nodes={[nodeA, nodeB, nodeC]}
        edges={[]}
        selection={{ kind: "nodes", nodes: [nodeA, nodeB, nodeC] }}
        onSelectionChange={vi.fn()}
      />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !cytoscapeState.latestCore) {
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

    cytoscapeState.latestCore.setSelectedNodeIDs(["node-a", "node-b", "node-c"]);

    const before = ["node-a", "node-b", "node-c"].map((id) => cytoscapeState.latestCore?.nodePosition(id));
    const beforeCenter = {
      x: (before.reduce((sum, point) => sum + (point?.x ?? 0), 0)) / before.length,
      y: (before.reduce((sum, point) => sum + (point?.y ?? 0), 0)) / before.length,
    };

    fireEvent.contextMenu(surface, { clientX: 120, clientY: 40 });
    fireEvent.click(await screen.findByRole("button", { name: "Cluster Nodes" }));

    const after = ["node-a", "node-b", "node-c"].map((id) => cytoscapeState.latestCore?.nodePosition(id));
    const afterCenter = {
      x: (after.reduce((sum, point) => sum + (point?.x ?? 0), 0)) / after.length,
      y: (after.reduce((sum, point) => sum + (point?.y ?? 0), 0)) / after.length,
    };

    expect(after.some((point, index) => point?.x !== before[index]?.x || point?.y !== before[index]?.y)).toBe(true);
    expect(afterCenter.x).toBeCloseTo(beforeCenter.x, 5);
    expect(afterCenter.y).toBeCloseTo(beforeCenter.y, 5);
  });

  it("preserves extant node positions when the graph expands", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const nodeC = makeVisibleNode({ id: "node-c", label: "Node C" });

    const { rerender } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    expect(cytoscapeState.latestCore).not.toBeNull();
    if (!cytoscapeState.latestCore) {
      return;
    }

    cytoscapeState.latestCore.setNodePosition("node-a", { x: 333, y: 444 });
    cytoscapeState.latestCore.setNodePosition("node-b", { x: 555, y: 666 });

    rerender(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB, nodeC]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    await waitFor(() => {
      expect(cytoscapeState.latestCore?.nodePosition("node-c")).not.toBeNull();
    });

    expect(cytoscapeState.latestCore.nodePosition("node-a")).toEqual({ x: 333, y: 444 });
    expect(cytoscapeState.latestCore.nodePosition("node-b")).toEqual({ x: 555, y: 666 });
  });
});
