import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, act } from "@testing-library/react";
import { useState, type PropsWithChildren } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AddressExplorerResponse, LiveHoldingsRefreshResponse } from "../../../../lib/types";
import { useSharedGraphNodeActions } from "../useSharedGraphNodeActions";
import {
  makeExplorerResponse,
  makeNode,
  makeVisibleNode,
} from "../../../../test-support/graphFixtures";

const apiMocks = vi.hoisted(() => ({
  addToBlocklist: vi.fn(),
  refreshLiveHoldings: vi.fn(),
  upsertAnnotation: vi.fn(),
}));

vi.mock("../../../../lib/api", () => ({
  addToBlocklist: apiMocks.addToBlocklist,
  refreshLiveHoldings: apiMocks.refreshLiveHoldings,
  upsertAnnotation: apiMocks.upsertAnnotation,
}));

function renderNodeActionsHook(initialGraph: AddressExplorerResponse) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });

  function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }

  const hook = renderHook(
    () => {
      const [graph, setGraph] = useState<AddressExplorerResponse | null>(initialGraph);
      const [statusText, setStatusText] = useState("");
      const actions = useSharedGraphNodeActions({
        graph,
        setGraph,
        setStatusText,
        queryClient,
        unavailableEmptyMessage: "No unavailable nodes.",
        onRefreshNodeSuccess: (rawNodeCount) => `Refreshed ${rawNodeCount} node(s).`,
        onRefreshUnavailableSuccess: (requestedCount, response) =>
          `Checked ${requestedCount} unavailable node(s); refreshed ${response.nodes.length}.`,
      });

      return { actions, graph, queryClient, statusText };
    },
    { wrapper: Wrapper }
  );

  return { queryClient, ...hook };
}

describe("useSharedGraphNodeActions", () => {
  beforeEach(() => {
    apiMocks.addToBlocklist.mockReset();
    apiMocks.refreshLiveHoldings.mockReset();
    apiMocks.upsertAnnotation.mockReset();
    Object.defineProperty(window.navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: vi.fn(),
      },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("refreshes unavailable nodes and merges live-value warnings into the current graph", async () => {
    const initialGraph = makeExplorerResponse({
      warnings: ["existing warning"],
      nodes: [
        makeNode({
          id: "node-a",
          metrics: {
            address: "thor1a",
            live_holdings_status: "error",
            stale: true,
          },
        }),
        makeNode({
          id: "node-b",
          metrics: {
            address: "thor1b",
            live_holdings_status: "unavailable",
          },
        }),
        makeNode({
          id: "node-c",
          metrics: {
            address: "thor1c",
            live_holdings_status: "available",
          },
        }),
      ],
    });
    const refreshResponse: LiveHoldingsRefreshResponse = {
      nodes: [
        makeNode({
          id: "node-a",
          metrics: {
            live_holdings_status: "available",
            live_holdings_available: true,
            live_holdings_usd_spot: 99,
          },
        }),
      ],
      warnings: ["provider warning"],
      refreshed_at: "2026-01-01T00:00:00Z",
    };
    apiMocks.refreshLiveHoldings.mockResolvedValue(refreshResponse);

    const { result } = renderNodeActionsHook(initialGraph);

    await act(async () => {
      await result.current.actions.onRefreshUnavailable();
    });

    expect(apiMocks.refreshLiveHoldings).toHaveBeenCalledWith([
      expect.objectContaining({ id: "node-a" }),
      expect.objectContaining({ id: "node-b" }),
    ]);
    expect(result.current.statusText).toBe("Checked 2 unavailable node(s); refreshed 1.");
    expect(result.current.graph?.warnings).toEqual(["existing warning", "provider warning"]);
    expect(result.current.graph?.nodes.find((node) => node.id === "node-a")?.metrics).toEqual({
      address: "thor1a",
      live_holdings_status: "available",
      stale: true,
      live_holdings_available: true,
      live_holdings_usd_spot: 99,
    });
  });

  it("invalidates the blocklist after removing a node from the graph", async () => {
    apiMocks.addToBlocklist.mockResolvedValue({ ok: true });

    const initialGraph = makeExplorerResponse({
      nodes: [makeNode({ id: "node-a", metrics: { address: "thor1a" } })],
    });
    const visibleNode = makeVisibleNode({
      id: "node-a",
      chain: "THOR",
      raw_node_ids: ["node-a"],
      metrics: { address: "thor1a" },
    });
    const { queryClient, result } = renderNodeActionsHook(initialGraph);
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue(undefined);

    await act(async () => {
      await result.current.actions.onRemoveNode(visibleNode);
    });

    expect(apiMocks.addToBlocklist).toHaveBeenCalledWith({
      address: "thor1a",
      reason: "Removed from graph",
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["blocklist"] });
    expect(result.current.statusText).toBe("Removed thor1a from graph.");
  });

  it("reports missing address fallbacks instead of trying to copy ambiguous nodes", async () => {
    const initialGraph = makeExplorerResponse({
      nodes: [
        makeNode({ id: "node-a", metrics: { address: "thor1a" } }),
        makeNode({ id: "node-b", metrics: { address: "bc1b" } }),
      ],
    });
    const ambiguousNode = makeVisibleNode({
      id: "cluster-node",
      chain: "THOR",
      raw_node_ids: ["node-a", "node-b"],
      metrics: { address: "" },
    });

    const { result } = renderNodeActionsHook(initialGraph);

    await act(async () => {
      await result.current.actions.onCopyAddress(ambiguousNode);
    });

    expect(result.current.statusText).toBe("Selected node does not resolve to a single address.");
    expect(window.navigator.clipboard.writeText).not.toHaveBeenCalled();
  });
});
