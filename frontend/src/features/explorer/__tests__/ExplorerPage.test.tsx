import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { makeEdge, makeExplorerResponse, makeNode } from "../../../test-support/graphFixtures";
import { RouterProvider } from "../../../app/router";
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

  it("suggests labeled addresses and names what the typed address is", async () => {
    apiMocks.listAnnotations.mockResolvedValue([
      {
        id: 1,
        address: "thor1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
        normalized_address: "thor1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
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

    const { container } = renderPage();

    // Labeled addresses are offered as suggestions on the address field;
    // other annotation kinds are not.
    await waitFor(() => expect(container.querySelector('datalist option[value="thor1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]')).not.toBeNull());
    expect(container.querySelector('datalist option[value="thor1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]')?.getAttribute("label")).toBe("Treasury Hot Wallet");
    expect(container.querySelector('datalist option[value="thor1ignore"]')).toBeNull();

    const address = screen.getByPlaceholderText("thor1...") as HTMLInputElement;
    expect(address.getAttribute("list")).toBe(container.querySelector("datalist")?.id);
    fireEvent.change(address, { target: { value: "thor1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq" } });
    expect(screen.getByText("Treasury Hot Wallet, THOR address")).toBeTruthy();
  });

  it("explores an address sent from search or another page", async () => {
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.listAddressExplorerRuns.mockResolvedValue([]);
    apiMocks.buildAddressExplorer.mockReturnValue(new Promise(() => {}));
    window.location.hash = "#explorer?address=thor1linked";

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <RouterProvider>
        <QueryClientProvider client={queryClient}>
          <ExplorerPage />
        </QueryClientProvider>
      </RouterProvider>
    );

    await waitFor(() => expect(apiMocks.buildAddressExplorer).toHaveBeenCalled());
    expect(apiMocks.buildAddressExplorer.mock.calls[0][0]).toMatchObject({ address: "thor1linked", mode: "preview" });
    expect((screen.getByPlaceholderText("thor1...") as HTMLInputElement).value).toBe("thor1linked");
    // The parameters are consumed, so reloading does not explore again.
    expect(window.location.hash).toBe("#explorer");
    window.location.hash = "";
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
    await screen.findByRole("button", { name: "Open a file…" });

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
    // The header names the explored address and how it was loaded.
    expect(screen.getByText(/^thor1saved, oldest first/)).toBeTruthy();
    expect((screen.getByPlaceholderText("thor1...") as HTMLInputElement).value).toBe("thor1saved");
    expect((screen.getByLabelText("Min USD") as HTMLInputElement).value).toBe("42");
    expect((screen.getByLabelText("Batch size") as HTMLInputElement).value).toBe("7");
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
    await screen.findByRole("button", { name: "Open a file…" });

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
    fireEvent.click(await screen.findByTitle("Fullscreen (F)"));

    await waitFor(() => {
      expect(screen.queryByText("Explorer provider warning")).toBeNull();
    });
  });
});
