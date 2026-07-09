import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GraphCanvas } from "../../GraphCanvas";
import { makeVisibleNode } from "../../../../test-support/graphFixtures";
import { mockCytoscapeState } from "../../../../test-support/cytoscapeMock";
import { DEFAULT_GRAPH_LABEL_MAX_WIDTH_PX } from "../constants";

vi.mock("cytoscape", async () => {
  const { mockCytoscapeFactory } = await import("../../../../test-support/cytoscapeMock");
  return { default: mockCytoscapeFactory };
});

vi.mock("elkjs/lib/elk.bundled.js", async () => {
  const { MockELK } = await import("../../../../test-support/cytoscapeMock");
  return { default: MockELK };
});

function sizeSurface(surface: HTMLDivElement) {
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
}

describe("GraphCanvas multi-node context menu", () => {
  beforeEach(() => {
    mockCytoscapeState.latestCore = null;
    window.localStorage.clear();
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
    sizeSurface(surface);

    expect(mockCytoscapeState.latestCore).not.toBeNull();
    mockCytoscapeState.latestCore?.setSelectedNodeIDs(["node-a", "node-b"]);

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
    sizeSurface(surface);

    mockCytoscapeState.latestCore?.setSelectedNodeIDs(["node-a", "node-b"]);

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
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    mockCytoscapeState.latestCore.setSelectedNodeIDs(["node-a", "node-b", "node-c"]);

    const before = ["node-a", "node-b", "node-c"].map((id) => mockCytoscapeState.latestCore?.nodePosition(id));
    const beforeCenter = {
      x: (before.reduce((sum, point) => sum + (point?.x ?? 0), 0)) / before.length,
      y: (before.reduce((sum, point) => sum + (point?.y ?? 0), 0)) / before.length,
    };

    fireEvent.contextMenu(surface, { clientX: 120, clientY: 40 });
    fireEvent.click(await screen.findByRole("button", { name: "Cluster Nodes" }));

    const after = ["node-a", "node-b", "node-c"].map((id) => mockCytoscapeState.latestCore?.nodePosition(id));
    const afterCenter = {
      x: (after.reduce((sum, point) => sum + (point?.x ?? 0), 0)) / after.length,
      y: (after.reduce((sum, point) => sum + (point?.y ?? 0), 0)) / after.length,
    };

    expect(after.some((point, index) => point?.x !== before[index]?.x || point?.y !== before[index]?.y)).toBe(true);
    expect(afterCenter.x).toBeCloseTo(beforeCenter.x, 5);
    expect(afterCenter.y).toBeCloseTo(beforeCenter.y, 5);
  });

  it("keeps clustered nodes loose enough for node labels to stay readable", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "THOR Treasury Alpha" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "THOR Treasury Beta" });
    const nodeC = makeVisibleNode({ id: "node-c", label: "THOR Treasury Gamma" });

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
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    mockCytoscapeState.latestCore.setSelectedNodeIDs(["node-a", "node-b", "node-c"]);

    fireEvent.contextMenu(surface, { clientX: 120, clientY: 40 });
    fireEvent.click(await screen.findByRole("button", { name: "Cluster Nodes" }));

    const after = ["node-a", "node-b", "node-c"]
      .map((id) => mockCytoscapeState.latestCore?.nodePosition(id))
      .filter((point): point is { x: number; y: number } => Boolean(point));
    const distances = after.flatMap((point, index) =>
      after.slice(index + 1).map((other) => Math.hypot(other.x - point.x, other.y - point.y))
    );

    expect(Math.min(...distances)).toBeGreaterThanOrEqual(DEFAULT_GRAPH_LABEL_MAX_WIDTH_PX);
  });

  it("preserves extant node positions when the graph expands", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const nodeC = makeVisibleNode({ id: "node-c", label: "Node C" });

    const { rerender } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    expect(mockCytoscapeState.latestCore).not.toBeNull();
    if (!mockCytoscapeState.latestCore) {
      return;
    }

    mockCytoscapeState.latestCore.setNodePosition("node-a", { x: 333, y: 444 });
    mockCytoscapeState.latestCore.setNodePosition("node-b", { x: 555, y: 666 });

    rerender(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB, nodeC]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    await waitFor(() => {
      expect(mockCytoscapeState.latestCore?.nodePosition("node-c")).not.toBeNull();
    });

    expect(mockCytoscapeState.latestCore.nodePosition("node-a")).toEqual({ x: 333, y: 444 });
    expect(mockCytoscapeState.latestCore.nodePosition("node-b")).toEqual({ x: 555, y: 666 });
  });

  it("captures current node positions and viewport when saving graph state", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const onSaveState = vi.fn();

    render(
      <GraphCanvas
        mode="explorer"
        nodes={[nodeA, nodeB]}
        edges={[]}
        selection={null}
        onSelectionChange={vi.fn()}
        onSaveState={onSaveState}
      />
    );

    expect(mockCytoscapeState.latestCore).not.toBeNull();
    if (!mockCytoscapeState.latestCore) {
      return;
    }

    mockCytoscapeState.latestCore.setNodePosition("node-a", { x: 333, y: 444 });
    mockCytoscapeState.latestCore.setNodePosition("node-b", { x: 555, y: 666 });
    mockCytoscapeState.latestCore.zoom(1.75);
    mockCytoscapeState.latestCore.pan({ x: 90, y: 120 });

    fireEvent.click(screen.getByTitle("Save graph state"));

    expect(onSaveState).toHaveBeenCalledWith({
      node_positions: {
        "node-a": { x: 333, y: 444 },
        "node-b": { x: 555, y: 666 },
      },
      viewport: {
        zoom: 1.75,
        pan: { x: 90, y: 120 },
      },
    });
  });

  it("restores saved node positions and viewport on reset", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    render(
      <GraphCanvas
        mode="explorer"
        nodes={[nodeA, nodeB]}
        edges={[]}
        selection={null}
        onSelectionChange={vi.fn()}
        graphResetKey={1}
        savedCanvasState={{
          node_positions: {
            "node-a": { x: 333, y: 444 },
            "node-b": { x: 555, y: 666 },
          },
          viewport: {
            zoom: 1.75,
            pan: { x: 90, y: 120 },
          },
        }}
      />
    );

    await waitFor(() => {
      expect(mockCytoscapeState.latestCore?.nodePosition("node-a")).toEqual({ x: 333, y: 444 });
    });

    expect(mockCytoscapeState.latestCore?.nodePosition("node-b")).toEqual({ x: 555, y: 666 });
    expect(mockCytoscapeState.latestCore?.zoom()).toBe(1.75);
    expect(mockCytoscapeState.latestCore?.pan()).toEqual({ x: 90, y: 120 });
  });

  it("renders live holdings labels for nodes that have inline live values", async () => {
    const nodeA = makeVisibleNode({
      id: "node-a",
      label: "Node A",
      displayLabel: "Node A",
      live_holdings_label: "$1.2M",
      live_holdings_status: "available",
    });

    render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    await waitFor(() => {
      expect(screen.getByText("$1.2M")).toBeTruthy();
    });
  });

  it("pans on MacBook-style trackpad wheel gestures instead of zooming", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 18,
      deltaY: 14,
    });

    expect(mockCytoscapeState.latestCore.pan()).not.toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).toBe(1);
  });

  it("pans on large vertical-only trackpad drags instead of treating them as wheel zoom", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 1.5,
      deltaY: 56.3,
    });

    expect(mockCytoscapeState.latestCore.pan()).not.toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).toBe(1);
  });

  it("keeps mouse-wheel zoom behavior for discrete wheel input", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaY: 120,
    });

    expect(mockCytoscapeState.latestCore.pan()).toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).not.toBe(1);
  });

  it("zooms for mouse wheel with typical macOS small delta (BUG: was panning)", async () => {
    // On macOS with medium scroll speed, a single mouse wheel notch often produces
    // deltaY ~40-80, pixel-mode, integer, vertical-only — identical signature to the
    // old "trackpad pan" heuristic. This MUST zoom, not pan.
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 0,
      deltaY: 53,
    });

    expect(mockCytoscapeState.latestCore.pan()).toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).not.toBe(1);
  });

  it("keeps pinch-to-zoom behavior for trackpad zoom gestures", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      ctrlKey: true,
      deltaMode: 0,
      deltaY: 12,
    });

    expect(mockCytoscapeState.latestCore.pan()).toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).not.toBe(1);
  });

  it("zooms more aggressively for pinch gestures than the default wheel sensitivity", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      ctrlKey: true,
      deltaMode: 0,
      deltaY: -18,
    });

    expect(mockCytoscapeState.latestCore.pan()).toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).toBeGreaterThanOrEqual(1.06);
  });

  it("zooms for Chrome mouse wheel events identified by wheelDelta multiple of 120", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    // Simulate a Chrome mouse wheel event: pixel-mode, small delta, but wheelDelta is a multiple of 120.
    // Without the wheelDelta check, this small delta would be misclassified as trackpad pan.
    const wheelEvent = new WheelEvent("wheel", {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 0,
      deltaY: 50,
      bubbles: true,
      cancelable: true,
    });
    Object.defineProperty(wheelEvent, "wheelDelta", { value: -120 });
    surface.dispatchEvent(wheelEvent);

    expect(mockCytoscapeState.latestCore.pan()).toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).not.toBe(1);
  });

  it("pans for trackpad events even when wheelDelta is present but not a multiple of 120", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    // Trackpad in Chrome: wheelDelta is NOT a multiple of 120, and deltas have horizontal component
    const wheelEvent = new WheelEvent("wheel", {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 12,
      deltaY: 18,
      bubbles: true,
      cancelable: true,
    });
    Object.defineProperty(wheelEvent, "wheelDelta", { value: -54 });
    surface.dispatchEvent(wheelEvent);

    expect(mockCytoscapeState.latestCore.pan()).not.toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).toBe(1);
  });

  it("maintains trackpad pan through inertia scrolling beyond 200ms", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    // First trackpad event - small fractional delta with horizontal jitter (clearly trackpad)
    fireEvent.wheel(surface, {
      clientX: 240, clientY: 160, deltaMode: 0, deltaX: 0.8, deltaY: 8.5,
    });

    const panAfterFirst = { ...mockCytoscapeState.latestCore.pan() };
    expect(panAfterFirst).not.toEqual({ x: 0, y: 0 });

    // Simulate 250ms passing (was beyond old 180ms lock, within new 400ms lock)
    vi.useFakeTimers();
    vi.advanceTimersByTime(250);

    // Second event - large vertical integer delta that would be ambiguous without gesture lock
    fireEvent.wheel(surface, {
      clientX: 240, clientY: 160, deltaMode: 0, deltaX: 0, deltaY: 140,
    });

    vi.useRealTimers();

    // Should have continued panning (not zoomed) thanks to the extended gesture lock
    expect(mockCytoscapeState.latestCore.pan()).not.toEqual(panAfterFirst);
    expect(mockCytoscapeState.latestCore.zoom()).toBe(1);
  });

  it("zooms for a physical mouse wheel even immediately after a trackpad pan gesture", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 12,
      deltaY: 18,
    });

    expect(mockCytoscapeState.latestCore.pan()).not.toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).toBe(1);

    const mouseWheelEvent = new WheelEvent("wheel", {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 0,
      deltaY: 50,
      bubbles: true,
      cancelable: true,
    });
    Object.defineProperty(mouseWheelEvent, "wheelDelta", { value: -120 });
    surface.dispatchEvent(mouseWheelEvent);

    expect(mockCytoscapeState.latestCore.pan()).not.toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).not.toBe(1);
  });

  it("zooms for mouse wheel with macOS scroll-smoothed fractional deltaY (BUG: was panning)", async () => {
    // On macOS, scroll acceleration smooths mouse wheel events into fractional deltaY values.
    // A standard mouse wheel notch can produce e.g. deltaY: 53.333 with deltaX: 0.
    // The fractional check was misclassifying this as trackpad. It should zoom.
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    // Mouse wheel on macOS with scroll smoothing: fractional deltaY, zero deltaX, no wheelDelta in JSDOM
    fireEvent.wheel(surface, {
      clientX: 240,
      clientY: 160,
      deltaMode: 0,
      deltaX: 0,
      deltaY: 53.333,
    });

    expect(mockCytoscapeState.latestCore.pan()).toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).not.toBe(1);
  });
});
