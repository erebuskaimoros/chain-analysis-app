import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GraphCanvas } from "../../GraphCanvas";
import { makeVisibleEdge, makeVisibleNode } from "../../../../test-support/graphFixtures";
import { mockCytoscapeState, type MockNodeElement } from "../../../../test-support/cytoscapeMock";

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

describe("GraphCanvas map improvements", () => {
  beforeEach(() => {
    mockCytoscapeState.latestCore = null;
    mockCytoscapeState.lastOptions = null;
    window.localStorage.clear();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("initializes cytoscape with bounded zoom limits (BUG: zoom was unbounded)", () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });

    render(<GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />);

    expect(mockCytoscapeState.lastOptions).not.toBeNull();
    expect(mockCytoscapeState.lastOptions?.minZoom).toBe(0.05);
    expect(mockCytoscapeState.lastOptions?.maxZoom).toBe(10);
  });

  it("selects a plain node immediately on tap (BUG: every click waited 320ms)", () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const onSelectionChange = vi.fn();

    render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={onSelectionChange} />
    );

    const core = mockCytoscapeState.latestCore;
    expect(core).not.toBeNull();
    core?.emit("tap", { target: core.getElementById("node-a") });

    expect(onSelectionChange).toHaveBeenCalledWith({ kind: "node", node: nodeA });
  });

  it("still defers taps on nodes whose single-tap runs a primary action", () => {
    const actorNode = makeVisibleNode({ id: "actor-1", label: "Actor", kind: "actor", actor_ids: [7] });
    const onSelectionChange = vi.fn();
    const onNodePrimaryAction = vi.fn(() => true);

    render(
      <GraphCanvas
        mode="actor"
        nodes={[actorNode]}
        edges={[]}
        selection={null}
        onSelectionChange={onSelectionChange}
        onNodePrimaryAction={onNodePrimaryAction}
        nodeHasPrimaryAction={(node) => node.kind === "actor"}
      />
    );

    const core = mockCytoscapeState.latestCore;
    expect(core).not.toBeNull();

    vi.useFakeTimers();
    core?.emit("tap", { target: core.getElementById("actor-1") });

    expect(onNodePrimaryAction).not.toHaveBeenCalled();
    expect(onSelectionChange).not.toHaveBeenCalled();

    vi.advanceTimersByTime(340);

    expect(onNodePrimaryAction).toHaveBeenCalledWith(actorNode);
    expect(onSelectionChange).not.toHaveBeenCalled();
  });

  it("anchors newly expanded nodes next to their neighbors without moving existing nodes", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const nodeC = makeVisibleNode({ id: "node-c", label: "Node C" });
    const edgeAB = makeVisibleEdge({ source: "node-a", target: "node-b" });
    const edgeBC = makeVisibleEdge({ source: "node-b", target: "node-c" });

    const { rerender } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[edgeAB]} selection={null} onSelectionChange={vi.fn()} />
    );

    const core = mockCytoscapeState.latestCore;
    expect(core).not.toBeNull();
    if (!core) {
      return;
    }

    await waitFor(() => {
      expect(core.nodePosition("node-a")).toEqual({ x: 40, y: 40 });
    });

    core.setNodePosition("node-a", { x: 100, y: 100 });
    core.setNodePosition("node-b", { x: 400, y: 100 });

    rerender(
      <GraphCanvas
        mode="explorer"
        nodes={[nodeA, nodeB, nodeC]}
        edges={[edgeAB, edgeBC]}
        selection={null}
        onSelectionChange={vi.fn()}
      />
    );

    // Existing nodes stay exactly where the user left them.
    expect(core.nodePosition("node-a")).toEqual({ x: 100, y: 100 });
    expect(core.nodePosition("node-b")).toEqual({ x: 400, y: 100 });

    // The new node grows out of its neighbor instead of landing at a fresh
    // ELK origin far from (or on top of) the preserved drawing.
    expect(core.nodePosition("node-c")).toEqual({ x: 580, y: 100 });
  });

  it("removes departed elements during incremental sync without touching survivors", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });

    const { rerender } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const core = mockCytoscapeState.latestCore;
    if (!core) {
      expect(core).not.toBeNull();
      return;
    }

    await waitFor(() => {
      expect(core.nodePosition("node-a")).toEqual({ x: 40, y: 40 });
    });
    core.setNodePosition("node-a", { x: 250, y: 260 });

    rerender(<GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />);

    expect(core.nodePosition("node-b")).toBeNull();
    expect(core.nodePosition("node-a")).toEqual({ x: 250, y: 260 });
  });

  it("highlights search matches and centers the active one", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Treasury Alpha", displayLabel: "Treasury Alpha" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Binance Hot", displayLabel: "Binance Hot" });

    const { container, findByText } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const input = container.querySelector(".graph-search input") as HTMLInputElement | null;
    expect(input).not.toBeNull();
    if (!input) {
      return;
    }

    fireEvent.change(input, { target: { value: "treasury" } });

    expect(await findByText("1/1")).toBeTruthy();

    const core = mockCytoscapeState.latestCore;
    const matched = core?.getElementById("node-a") as MockNodeElement;
    const other = core?.getElementById("node-b") as MockNodeElement;
    expect(matched.hasClass("graph-search-match")).toBe(true);
    expect(matched.hasClass("graph-search-active")).toBe(true);
    expect(other.hasClass("graph-search-match")).toBe(false);
    expect(core?.lastCenteredID).toBe("node-a");
  });

  it("dims elements outside the selected node's neighborhood", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const nodeC = makeVisibleNode({ id: "node-c", label: "Node C" });
    const edgeAB = makeVisibleEdge({ source: "node-a", target: "node-b" });

    render(
      <GraphCanvas
        mode="explorer"
        nodes={[nodeA, nodeB, nodeC]}
        edges={[edgeAB]}
        selection={{ kind: "node", node: nodeA }}
        onSelectionChange={vi.fn()}
      />
    );

    const core = mockCytoscapeState.latestCore;
    expect(core).not.toBeNull();
    if (!core) {
      return;
    }

    const selected = core.getElementById("node-a") as MockNodeElement;
    const neighbor = core.getElementById("node-b") as MockNodeElement;
    const outsider = core.getElementById("node-c") as MockNodeElement;

    expect(selected.hasClass("graph-dimmed")).toBe(false);
    expect(neighbor.hasClass("graph-dimmed")).toBe(false);
    expect(outsider.hasClass("graph-dimmed")).toBe(true);
    expect(selected.hasClass("graph-highlighted")).toBe(true);
  });

  it("always pans the wheel when the stored preference is 'pan'", () => {
    window.localStorage.setItem("graph-canvas-wheel-mode", "pan");
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

    // Discrete mouse-wheel signature that would zoom under the auto heuristic.
    fireEvent.wheel(surface, { clientX: 240, clientY: 160, deltaMode: 0, deltaX: 0, deltaY: 120 });

    expect(mockCytoscapeState.latestCore.pan()).not.toEqual({ x: 0, y: 0 });
    expect(mockCytoscapeState.latestCore.zoom()).toBe(1);
  });

  it("pans with space+drag instead of box-selecting", () => {
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

    fireEvent.keyDown(window, { key: " ", code: "Space" });
    fireEvent.mouseDown(surface, { button: 0, clientX: 200, clientY: 200 });
    fireEvent.mouseMove(window, { clientX: 260, clientY: 240 });

    expect(mockCytoscapeState.latestCore.pan()).toEqual({ x: 60, y: 40 });

    fireEvent.mouseUp(window, { button: 0 });
    fireEvent.keyUp(window, { key: " ", code: "Space" });
  });

  it("continues a box-select drag when the pointer leaves the canvas (BUG: drag died at edge)", () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const nodeB = makeVisibleNode({ id: "node-b", label: "Node B" });
    const onSelectionChange = vi.fn();

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA, nodeB]} edges={[]} selection={null} onSelectionChange={onSelectionChange} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    expect(surface).not.toBeNull();
    if (!surface || !mockCytoscapeState.latestCore) {
      return;
    }
    sizeSurface(surface);

    // Start on empty canvas space (mock nodes sit at x=40/120, y=40).
    fireEvent.mouseDown(surface, { button: 0, clientX: 300, clientY: 300 });
    // Drag beyond the canvas bounds — previously mouseleave cancelled the gesture.
    fireEvent.mouseMove(window, { clientX: 10, clientY: 10 });
    fireEvent.mouseUp(window, { button: 0, clientX: 10, clientY: 10 });

    expect(onSelectionChange).toHaveBeenCalledWith({
      kind: "nodes",
      nodes: [nodeA, nodeB],
    });
  });

  it("does not hijack mousedown on overlay widgets like the search box (BUG: input could not focus)", () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A" });
    const onSelectionChange = vi.fn();

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={onSelectionChange} />
    );

    const surface = container.querySelector(".graph-surface") as HTMLDivElement | null;
    const input = container.querySelector(".graph-search input") as HTMLInputElement | null;
    expect(surface).not.toBeNull();
    expect(input).not.toBeNull();
    if (!surface || !input) {
      return;
    }
    sizeSurface(surface);

    // Mousedown on the search input must not be treated as a box-select start:
    // preventDefault there would block the input from ever taking focus.
    const event = new MouseEvent("mousedown", { button: 0, bubbles: true, cancelable: true });
    input.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);

    // And dragging afterwards must not produce a box selection.
    fireEvent.mouseMove(window, { clientX: 200, clientY: 200 });
    fireEvent.mouseUp(window, { button: 0, clientX: 200, clientY: 200 });
    expect(onSelectionChange).not.toHaveBeenCalled();
  });

  it("shows a hover card after lingering over a node", async () => {
    const nodeA = makeVisibleNode({ id: "node-a", label: "Node A", displayLabel: "Node A" });

    const { container } = render(
      <GraphCanvas mode="explorer" nodes={[nodeA]} edges={[]} selection={null} onSelectionChange={vi.fn()} />
    );

    const core = mockCytoscapeState.latestCore;
    expect(core).not.toBeNull();
    if (!core) {
      return;
    }

    vi.useFakeTimers();
    act(() => {
      core.emit("mouseover", { target: core.getElementById("node-a") });
    });
    act(() => {
      vi.advanceTimersByTime(200);
    });
    vi.useRealTimers();

    await waitFor(() => {
      expect(container.querySelector(".graph-hover-card")).not.toBeNull();
    });
    expect(container.querySelector(".graph-hover-card")?.textContent).toContain("addr-node-a");
  });
});
