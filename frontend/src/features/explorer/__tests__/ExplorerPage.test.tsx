import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { makeEdge, makeExplorerResponse, makeNode } from "../../../test-support/graphFixtures";
import { ExplorerPage } from "../ExplorerPage";

const apiMocks = vi.hoisted(() => ({
  listAnnotations: vi.fn(),
  listBlocklist: vi.fn(),
  listAddressExplorerRuns: vi.fn(),
  buildAddressExplorer: vi.fn(),
  deleteAddressExplorerRun: vi.fn(),
  expandActorGraph: vi.fn(),
  lookupAction: vi.fn(),
  refreshLiveHoldings: vi.fn(),
  upsertAnnotation: vi.fn(),
  addToBlocklist: vi.fn(),
}));

vi.mock("../../../lib/api", () => ({
  listAnnotations: apiMocks.listAnnotations,
  listBlocklist: apiMocks.listBlocklist,
  listAddressExplorerRuns: apiMocks.listAddressExplorerRuns,
  buildAddressExplorer: apiMocks.buildAddressExplorer,
  deleteAddressExplorerRun: apiMocks.deleteAddressExplorerRun,
  expandActorGraph: apiMocks.expandActorGraph,
  lookupAction: apiMocks.lookupAction,
  refreshLiveHoldings: apiMocks.refreshLiveHoldings,
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
      <ExplorerPage />
    </QueryClientProvider>
  );
}

describe("ExplorerPage", () => {
  afterEach(() => {
    cleanup();
    Object.values(apiMocks).forEach((mockFn) => mockFn.mockReset());
  });

  it("fills the explorer address field from a saved label annotation", async () => {
    apiMocks.listAnnotations.mockResolvedValue([
      {
        id: 1,
        address: "thor1treasury",
        normalized_address: "thor1treasury",
        kind: "label",
        value: "Treasury Hot Wallet",
        created_at: "2026-03-11T12:00:00Z",
      },
      {
        id: 2,
        address: "thor1ignore",
        normalized_address: "thor1ignore",
        kind: "notes",
        value: "Ignore me",
        created_at: "2026-03-11T12:05:00Z",
      },
    ]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.listAddressExplorerRuns.mockResolvedValue([]);

    renderPage();

    await screen.findByRole("option", { name: "Treasury Hot Wallet · thor1treasury" });

    fireEvent.change(screen.getByRole("combobox", { name: "Named Addresses" }), {
      target: { value: "1" },
    });

    await waitFor(() =>
      expect((screen.getByPlaceholderText("thor1...") as HTMLInputElement).value).toBe("thor1treasury")
    );
    expect((screen.getByRole("combobox", { name: "Named Addresses" }) as HTMLSelectElement).value).toBe("1");
    expect(screen.queryByRole("option", { name: /Ignore me/ })).toBeNull();
  });

  it("loads a saved explorer graph state from disk", async () => {
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.listAddressExplorerRuns.mockResolvedValue([]);

    const graph = makeExplorerResponse({
      address: "thor1saved",
      nodes: [],
      edges: [],
      supporting_actions: [],
      active_chains: ["THOR"],
      total_estimate: 12,
      query: {
        address: "thor1saved",
        min_usd: 42,
        batch_size: 7,
        direction: "oldest",
      },
    });

    const { container } = renderPage();
    await screen.findByRole("button", { name: "Load saved state" });

    const input = container.querySelector('input[type="file"]');
    expect(input).not.toBeNull();

    const file = new File(
      [
        JSON.stringify({
          schema_version: 1,
          kind: "address-explorer",
          exported_at: "2026-03-11T13:00:00Z",
          request: {
            address: "thor1saved",
            flow_types: ["transfers", "swaps", "bonds"],
            min_usd: 42,
            mode: "graph",
            direction: "oldest",
            offset: 0,
            batch_size: 7,
          },
          preview: graph,
          ui_state: {
            form: {
              address: "thor1saved",
              min_usd: "42",
              batch_size: 7,
            },
          },
          graph,
        }),
      ],
      "explorer-state.json",
      { type: "application/json" }
    );

    fireEvent.change(input as HTMLInputElement, { target: { files: [file] } });

    await waitFor(() => expect(screen.getByText("Loaded graph state from explorer-state.json.")).toBeTruthy());
    expect(screen.getByRole("heading", { name: "Explorer Graph" })).toBeTruthy();
    expect((screen.getByPlaceholderText("thor1...") as HTMLInputElement).value).toBe("thor1saved");
    expect((screen.getByLabelText("Min USD") as HTMLInputElement).value).toBe("42");
    expect((screen.getByLabelText("Batch Size") as HTMLInputElement).value).toBe("7");
  });

  it("hides graph-build warnings when the explorer graph enters fullscreen mode", async () => {
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.listAddressExplorerRuns.mockResolvedValue([]);

    const graph = makeExplorerResponse({
      address: "thor1saved",
      nodes: [
        makeNode({
          id: "seed",
          kind: "explorer_target",
          label: "thor1saved",
          metrics: { address: "thor1saved" },
        }),
        makeNode({
          id: "counterparty",
          kind: "external_address",
          label: "Counterparty",
          chain: "BTC",
          metrics: { address: "bc1counterparty" },
        }),
      ],
      edges: [
        makeEdge({
          id: "visible-edge",
          from: "seed",
          to: "counterparty",
        }),
      ],
      supporting_actions: [],
      warnings: ["Explorer provider warning"],
      active_chains: ["THOR"],
      total_estimate: 12,
      query: {
        address: "thor1saved",
        min_usd: 42,
        batch_size: 7,
        direction: "oldest",
      },
    });

    const { container } = renderPage();
    await screen.findByRole("button", { name: "Load saved state" });

    const input = container.querySelector('input[type="file"]');
    expect(input).not.toBeNull();

    const file = new File(
      [
        JSON.stringify({
          schema_version: 1,
          kind: "address-explorer",
          exported_at: "2026-03-11T13:00:00Z",
          request: {
            address: "thor1saved",
            flow_types: ["transfers", "swaps", "bonds"],
            min_usd: 42,
            mode: "graph",
            direction: "oldest",
            offset: 0,
            batch_size: 7,
          },
          preview: graph,
          ui_state: {
            form: {
              address: "thor1saved",
              min_usd: "42",
              batch_size: 7,
            },
          },
          graph,
        }),
      ],
      "explorer-warnings-state.json",
      { type: "application/json" }
    );

    fireEvent.change(input as HTMLInputElement, { target: { files: [file] } });

    await screen.findByText("Explorer provider warning");
    fireEvent.click(screen.getByTitle("Fullscreen (F)"));

    await waitFor(() => {
      expect(screen.queryByText("Explorer provider warning")).toBeNull();
    });
  });
});
