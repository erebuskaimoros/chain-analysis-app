import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeActor, makeActorGraphResponse, makeEdge, makeNode } from "../../../test-support/graphFixtures";
import { ActorGraphPage } from "../ActorGraphPage";

const apiMocks = vi.hoisted(() => ({
  listActors: vi.fn(),
  listActorGraphRuns: vi.fn(),
  buildActorGraph: vi.fn(),
  deleteActorGraphRun: vi.fn(),
  expandActorGraph: vi.fn(),
  lookupAction: vi.fn(),
  refreshLiveHoldings: vi.fn(),
  listAnnotations: vi.fn(),
  listBlocklist: vi.fn(),
  upsertAnnotation: vi.fn(),
  addToBlocklist: vi.fn(),
  listGraphStates: vi.fn(),
  createGraphState: vi.fn(),
  getGraphState: vi.fn(),
  deleteGraphState: vi.fn(),
  refreshLiveHoldingsInBackground: vi.fn(),
}));

vi.mock("../../../lib/api", () => ({
  listActors: apiMocks.listActors,
  listActorGraphRuns: apiMocks.listActorGraphRuns,
  buildActorGraph: apiMocks.buildActorGraph,
  deleteActorGraphRun: apiMocks.deleteActorGraphRun,
  expandActorGraph: apiMocks.expandActorGraph,
  lookupAction: apiMocks.lookupAction,
  refreshLiveHoldings: apiMocks.refreshLiveHoldings,
  listAnnotations: apiMocks.listAnnotations,
  listBlocklist: apiMocks.listBlocklist,
  upsertAnnotation: apiMocks.upsertAnnotation,
  addToBlocklist: apiMocks.addToBlocklist,
  listGraphStates: apiMocks.listGraphStates,
  createGraphState: apiMocks.createGraphState,
  getGraphState: apiMocks.getGraphState,
  deleteGraphState: apiMocks.deleteGraphState,
  refreshLiveHoldingsInBackground: apiMocks.refreshLiveHoldingsInBackground,
}));

vi.mock("../../shared/GraphCanvas", () => ({
  GraphCanvas: ({ onFullscreenChange }: { onFullscreenChange?: (value: boolean) => void }) => (
    <div data-testid="graph-canvas-mock">
      <button type="button" title="Fullscreen (F)" onClick={() => onFullscreenChange?.(true)}>
        Fullscreen
      </button>
    </div>
  ),
}));

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <ActorGraphPage />
    </QueryClientProvider>
  );
}

describe("ActorGraphPage", () => {
  beforeEach(() => {
    apiMocks.listGraphStates.mockResolvedValue([]);
  });

  afterEach(() => {
    cleanup();
    Object.values(apiMocks).forEach((mockFn) => mockFn.mockReset());
  });

  it("loads a saved actor graph state from disk", async () => {
    const actor = makeActor({ id: 7, name: "Treasury", addresses: [] });
    const graph = makeActorGraphResponse({
      actors: [actor],
      nodes: [],
      edges: [],
      supporting_actions: [],
      query: {
        actor_ids: [7],
        start_time: "2026-02-01T00:00:00Z",
        end_time: "2026-02-03T00:00:00Z",
        max_hops: 3,
        min_usd: 25,
      },
    });

    apiMocks.listActors.mockResolvedValue([actor]);
    apiMocks.listActorGraphRuns.mockResolvedValue([]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.refreshLiveHoldingsInBackground.mockResolvedValue({
      nodes: graph.nodes,
      warnings: [],
      refreshed_at: "2026-03-18T12:00:00Z",
    });

    const { container } = renderPage();
    await screen.findByRole("button", { name: "Load saved state" });

    const input = container.querySelector('input[type="file"]');
    expect(input).not.toBeNull();

    const file = new File(
      [
        JSON.stringify({
          schema_version: 1,
          kind: "actor-graph",
          exported_at: "2026-03-11T12:00:00Z",
          request: {
            actor_ids: [7],
            start_time: "2026-02-01T00:00:00Z",
            end_time: "2026-02-03T00:00:00Z",
            max_hops: 3,
            flow_types: ["transfers", "swaps", "bonds"],
            min_usd: 25,
            collapse_external: false,
            display_mode: "graph",
          },
          ui_state: {
            selected_actor_ids: [7],
            form: {
              start_time: "2026-02-01T00:00",
              end_time: "2026-02-03T00:00",
              max_hops: 3,
              min_usd: "25",
            },
          },
          graph,
        }),
      ],
      "actor-state.json",
      { type: "application/json" }
    );

    fireEvent.change(input as HTMLInputElement, { target: { files: [file] } });

    await waitFor(() => expect(screen.getByText("Loaded graph state from actor-state.json.")).toBeTruthy());
    expect(screen.getByRole("heading", { name: "Current Flow Graph" })).toBeTruthy();
    expect((screen.getByLabelText("Min USD") as HTMLInputElement).value).toBe("25");
    expect((screen.getByLabelText("Max Hops") as HTMLInputElement).value).toBe("3");
    expect((screen.getByLabelText("Start") as HTMLInputElement).value).toBe("2026-02-01T00:00");
    expect((screen.getByRole("checkbox", { name: /Treasury/ }) as HTMLInputElement).checked).toBe(true);
  });

  it("loads a saved graph state from the server list", async () => {
    const actor = makeActor({ id: 7, name: "Treasury", addresses: [] });
    const graph = makeActorGraphResponse({
      actors: [actor],
      nodes: [],
      edges: [],
      supporting_actions: [],
      query: {
        actor_ids: [7],
        start_time: "2026-02-01T00:00:00Z",
        end_time: "2026-02-03T00:00:00Z",
        max_hops: 3,
        min_usd: 25,
      },
    });
    const summary = {
      id: 3,
      kind: "actor-graph",
      name: "Treasury case",
      node_count: 0,
      edge_count: 0,
      created_at: "2026-03-01T00:00:00Z",
      updated_at: "2026-03-01T00:00:00Z",
    };

    apiMocks.listActors.mockResolvedValue([actor]);
    apiMocks.listActorGraphRuns.mockResolvedValue([]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.listGraphStates.mockResolvedValue([summary]);
    apiMocks.getGraphState.mockResolvedValue({
      ...summary,
      state: {
        schema_version: 1,
        kind: "actor-graph",
        exported_at: "2026-03-01T00:00:00Z",
        request: {
          actor_ids: [7],
          start_time: "2026-02-01T00:00:00Z",
          end_time: "2026-02-03T00:00:00Z",
          max_hops: 3,
          flow_types: ["transfers", "swaps", "bonds"],
          min_usd: 25,
          collapse_external: false,
          display_mode: "graph",
        },
        ui_state: {
          selected_actor_ids: [7],
          form: {
            start_time: "2026-02-01T00:00",
            end_time: "2026-02-03T00:00",
            max_hops: 3,
            min_usd: "25",
          },
        },
        graph,
      },
    });
    apiMocks.refreshLiveHoldingsInBackground.mockResolvedValue({
      nodes: graph.nodes,
      warnings: [],
      refreshed_at: "2026-03-18T12:00:00Z",
    });

    const { container } = renderPage();
    await screen.findByText(/Treasury case/);

    const select = Array.from(container.querySelectorAll("select")).find((element) =>
      element.textContent?.includes("Treasury case")
    );
    expect(select).not.toBeUndefined();
    fireEvent.change(select as HTMLSelectElement, { target: { value: "3" } });

    const loadButtons = screen.getAllByRole("button", { name: "Load" });
    fireEvent.click(loadButtons[loadButtons.length - 1]);

    await waitFor(() => expect(screen.getByText('Loaded graph state from "Treasury case".')).toBeTruthy());
    expect(apiMocks.getGraphState).toHaveBeenCalledWith(3);
    expect(screen.getByRole("heading", { name: "Current Flow Graph" })).toBeTruthy();
    expect((screen.getByLabelText("Min USD") as HTMLInputElement).value).toBe("25");
  });

  it("refreshes live holdings after loading a saved graph state", async () => {
    const actor = makeActor({ id: 7, name: "Treasury", addresses: [] });
    const graph = makeActorGraphResponse({
      actors: [actor],
      nodes: [
        makeNode({
          id: "external-address",
          kind: "external_address",
          chain: "ETH",
          metrics: { address: "0xwatch" },
        }),
      ],
      edges: [],
      supporting_actions: [],
      query: {
        actor_ids: [7],
        start_time: "2026-02-01T00:00:00Z",
        end_time: "2026-02-03T00:00:00Z",
        max_hops: 3,
        min_usd: 25,
      },
    });

    apiMocks.listActors.mockResolvedValue([actor]);
    apiMocks.listActorGraphRuns.mockResolvedValue([]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.refreshLiveHoldingsInBackground.mockResolvedValue({
      nodes: graph.nodes,
      warnings: [],
      refreshed_at: "2026-03-18T12:00:00Z",
    });

    const { container } = renderPage();
    await screen.findByRole("button", { name: "Load saved state" });

    const input = container.querySelector('input[type="file"]');
    expect(input).not.toBeNull();

    const file = new File(
      [
        JSON.stringify({
          schema_version: 1,
          kind: "actor-graph",
          exported_at: "2026-03-18T12:00:00Z",
          request: {
            actor_ids: [7],
            start_time: "2026-02-01T00:00:00Z",
            end_time: "2026-02-03T00:00:00Z",
            max_hops: 3,
            flow_types: ["transfers", "swaps", "bonds"],
            min_usd: 25,
            collapse_external: false,
            display_mode: "graph",
          },
          ui_state: {
            selected_actor_ids: [7],
            form: {
              start_time: "2026-02-01T00:00",
              end_time: "2026-02-03T00:00",
              max_hops: 3,
              min_usd: "25",
            },
          },
          graph,
        }),
      ],
      "actor-live-values-state.json",
      { type: "application/json" }
    );

    fireEvent.change(input as HTMLInputElement, { target: { files: [file] } });

    await screen.findByText("Loaded graph state from actor-live-values-state.json.");
    await waitFor(() => {
      expect(apiMocks.refreshLiveHoldingsInBackground).toHaveBeenCalledWith(graph.nodes, expect.objectContaining({ force: false }));
    });
  });

  it("renders the graph before background live holdings refresh finishes", async () => {
    const actor = makeActor({ id: 7, name: "Treasury", addresses: [] });
    const graph = makeActorGraphResponse({
      actors: [actor],
      nodes: [
        makeNode({
          id: "actor-address",
          kind: "actor_address",
          label: "Treasury Address",
          chain: "THOR",
          actor_ids: [7],
          metrics: { address: "thor1treasury" },
        }),
      ],
      edges: [],
      supporting_actions: [],
    });

    type RefreshResult = { nodes: typeof graph.nodes; warnings: string[]; refreshed_at: string };
    let resolveRefresh: (value: RefreshResult) => void = () => {};
    const refreshPromise = new Promise<RefreshResult>((resolve) => {
      resolveRefresh = resolve;
    });

    apiMocks.listActors.mockResolvedValue([actor]);
    apiMocks.listActorGraphRuns.mockResolvedValue([]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.buildActorGraph.mockResolvedValue(graph);
    apiMocks.refreshLiveHoldingsInBackground.mockReturnValue(refreshPromise);

    renderPage();

    await screen.findByText("Treasury");
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Build Graph" }));

    await screen.findByRole("heading", { name: "Current Flow Graph" });
    await waitFor(() => {
      expect(screen.getByText("Loaded 1 nodes and 0 edges for Treasury.")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Build Graph" })).toBeTruthy();
    });
    expect(apiMocks.refreshLiveHoldingsInBackground).toHaveBeenCalledWith(graph.nodes, expect.anything());

    resolveRefresh({
      nodes: graph.nodes,
      warnings: [],
      refreshed_at: "2026-03-12T12:00:00Z",
    });
  });

  it("merges partial live holdings from the background job before it finishes", async () => {
    const actor = makeActor({ id: 7, name: "Treasury", addresses: [] });
    const node = makeNode({
      id: "external-address",
      kind: "external_address",
      chain: "ETH",
      actor_ids: [7],
      metrics: { address: "0xwatch" },
    });
    const graph = makeActorGraphResponse({ actors: [actor], nodes: [node], edges: [], supporting_actions: [] });

    apiMocks.listActors.mockResolvedValue([actor]);
    apiMocks.listActorGraphRuns.mockResolvedValue([]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.buildActorGraph.mockResolvedValue(graph);
    apiMocks.refreshLiveHoldingsInBackground.mockImplementation(
      (_nodes: unknown, options: { onPartial?: (partial: unknown) => void }) => {
        options.onPartial?.({
          nodes: [{ id: "external-address", metrics: { address: "0xwatch", live_holdings_status: "available" } }],
          warnings: ["ETH tracker partial pass warning"],
          refreshed_at: "2026-03-19T12:00:00Z",
        });
        return new Promise(() => {});
      }
    );

    renderPage();
    await screen.findByText("Treasury");
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Build Graph" }));

    await screen.findByRole("heading", { name: "Current Flow Graph" });
    expect((await screen.findAllByText(/ETH tracker partial pass warning/)).length).toBeGreaterThan(0);
    expect(apiMocks.refreshLiveHoldingsInBackground).toHaveBeenCalledTimes(1);
  });

  it("reports the background job's final warnings", async () => {
    const actor = makeActor({ id: 7, name: "Treasury", addresses: [] });
    const graph = makeActorGraphResponse({
      actors: [actor],
      nodes: [
        makeNode({ id: "external-address", kind: "external_address", chain: "ETH", actor_ids: [7], metrics: { address: "0xwatch" } }),
      ],
      edges: [],
      supporting_actions: [],
    });

    apiMocks.listActors.mockResolvedValue([actor]);
    apiMocks.listActorGraphRuns.mockResolvedValue([]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.buildActorGraph.mockResolvedValue(graph);
    apiMocks.refreshLiveHoldingsInBackground.mockResolvedValue({
      nodes: [
        {
          id: "external-address",
          metrics: { address: "0xwatch", live_holdings_status: "error", live_holdings_error_kind: "banned" },
        },
      ],
      warnings: ["live holdings unavailable for 1 address nodes (banned 1)"],
      refreshed_at: "2026-03-19T12:00:01Z",
    });

    renderPage();
    await screen.findByText("Treasury");
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Build Graph" }));

    await screen.findByRole("heading", { name: "Current Flow Graph" });
    await waitFor(() => {
      expect(screen.getAllByText(/Background live holdings refresh finished\./).length).toBeGreaterThan(0);
    });
    expect(apiMocks.refreshLiveHoldingsInBackground).toHaveBeenCalledTimes(1);
  });

  it("hides graph-build warnings when the graph enters fullscreen mode", async () => {
    const actor = makeActor({ id: 7, name: "Treasury", addresses: [] });
    const graph = makeActorGraphResponse({
      actors: [actor],
      nodes: [
        makeNode({
          id: "actor-address",
          kind: "actor_address",
          label: "Treasury Address",
          actor_ids: [7],
          metrics: { address: "thor1treasury" },
        }),
        makeNode({
          id: "external-address",
          kind: "external_address",
          label: "Counterparty",
          chain: "BTC",
          metrics: { address: "bc1counterparty" },
        }),
      ],
      edges: [
        makeEdge({
          id: "visible-edge",
          from: "actor-address",
          to: "external-address",
        }),
      ],
      supporting_actions: [],
      warnings: ["Partial provider coverage"],
      query: {
        actor_ids: [7],
        start_time: "2026-02-01T00:00:00Z",
        end_time: "2026-02-03T00:00:00Z",
        max_hops: 3,
        min_usd: 25,
      },
    });

    apiMocks.listActors.mockResolvedValue([actor]);
    apiMocks.listActorGraphRuns.mockResolvedValue([]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.refreshLiveHoldingsInBackground.mockResolvedValue({
      nodes: graph.nodes,
      warnings: [],
      refreshed_at: "2026-03-18T12:00:00Z",
    });

    const { container } = renderPage();
    await screen.findByRole("button", { name: "Load saved state" });

    const input = container.querySelector('input[type="file"]');
    expect(input).not.toBeNull();

    const file = new File(
      [
        JSON.stringify({
          schema_version: 1,
          kind: "actor-graph",
          exported_at: "2026-03-11T12:00:00Z",
          request: {
            actor_ids: [7],
            start_time: "2026-02-01T00:00:00Z",
            end_time: "2026-02-03T00:00:00Z",
            max_hops: 3,
            flow_types: ["transfers", "swaps", "bonds"],
            min_usd: 25,
            collapse_external: false,
            display_mode: "graph",
          },
          ui_state: {
            selected_actor_ids: [7],
            form: {
              start_time: "2026-02-01T00:00",
              end_time: "2026-02-03T00:00",
              max_hops: 3,
              min_usd: "25",
            },
          },
          graph,
        }),
      ],
      "actor-warnings-state.json",
      { type: "application/json" }
    );

    fireEvent.change(input as HTMLInputElement, { target: { files: [file] } });

    await screen.findByText("Partial provider coverage");
    fireEvent.click(screen.getByTitle("Fullscreen (F)"));

    await waitFor(() => {
      expect(screen.queryByText("Partial provider coverage")).toBeNull();
    });
  });
});
