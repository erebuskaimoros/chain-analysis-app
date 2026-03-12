import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
