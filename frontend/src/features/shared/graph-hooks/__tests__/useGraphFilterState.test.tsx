import { renderHook, act } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useGraphFilterState } from "../useGraphFilterState";
import {
  makeEdge,
  makeExplorerResponse,
  makeNode,
  makeTransaction,
} from "../../../../test-support/graphFixtures";

describe("useGraphFilterState", () => {
  it("preserves constrained filters across graph expansions and pagination merges", () => {
    const initialGraph = makeExplorerResponse({
      nodes: [
        makeNode({ id: "btc-node", chain: "BTC", metrics: { address: "bc1node" } }),
        makeNode({ id: "thor-node", chain: "THOR", metrics: { address: "thor1node" } }),
      ],
      edges: [
        makeEdge({
          id: "initial-edge",
          from: "btc-node",
          to: "thor-node",
          transactions: [
            makeTransaction({ tx_id: "initial-a", time: "2026-01-01T00:00:00Z", usd_spot: 10 }),
            makeTransaction({ tx_id: "initial-b", time: "2026-01-03T00:00:00Z", usd_spot: 50 }),
          ],
        }),
      ],
    });
    const expandedGraph = makeExplorerResponse({
      nodes: [
        makeNode({ id: "btc-node-expanded", chain: "BTC", metrics: { address: "bc1expanded" } }),
        makeNode({ id: "thor-left", chain: "THOR", metrics: { address: "thor1left" } }),
        makeNode({ id: "thor-right", chain: "THOR", metrics: { address: "thor1right" } }),
      ],
      edges: [
        makeEdge({
          id: "expanded-edge",
          from: "thor-left",
          to: "thor-right",
          transactions: [
            makeTransaction({ tx_id: "expanded-a", time: "2026-01-02T00:00:00Z", usd_spot: 30 }),
            makeTransaction({ tx_id: "expanded-b", time: "2026-01-04T00:00:00Z", usd_spot: 80 }),
          ],
        }),
      ],
    });

    const { result } = renderHook(() => useGraphFilterState());

    act(() => {
      result.current.syncWithGraph(initialGraph, true);
    });
    act(() => {
      result.current.toggleChain("BTC", false);
      result.current.updateDate("startTime", "2026-01-02T00:00:00Z");
      result.current.updateNumber("maxTxnUSD", "40");
    });
    act(() => {
      result.current.syncWithGraph(expandedGraph, false);
    });

    expect(result.current.graphFilters.selectedChains).toEqual(["THOR"]);
    expect(result.current.graphFilters.startTime).toBe("2026-01-02T00:00:00.000Z");
    expect(result.current.graphFilters.maxTxnUSD).toBe(40);
    expect(result.current.filtersActive).toBe(true);
  });

  it("resets back to the full graph metadata on rebuilds and manual resets", () => {
    const initialGraph = makeExplorerResponse({
      nodes: [
        makeNode({ id: "thor-node", chain: "THOR", metrics: { address: "thor1node" } }),
        makeNode({ id: "btc-node", chain: "BTC", metrics: { address: "bc1node" } }),
      ],
      edges: [
        makeEdge({
          id: "initial-edge",
          from: "thor-node",
          to: "btc-node",
          transactions: [makeTransaction({ tx_id: "initial-a", time: "2026-01-01T00:00:00Z", usd_spot: 10 })],
        }),
      ],
    });
    const rebuiltGraph = makeExplorerResponse({
      nodes: [
        makeNode({ id: "eth-left", chain: "ETH", metrics: { address: "0xleft" } }),
        makeNode({ id: "eth-right", chain: "ETH", metrics: { address: "0xright" } }),
      ],
      edges: [
        makeEdge({
          id: "rebuilt-edge",
          from: "eth-left",
          to: "eth-right",
          transactions: [makeTransaction({ tx_id: "rebuilt-a", time: "2026-02-01T00:00:00Z", usd_spot: 90 })],
        }),
      ],
    });

    const { result } = renderHook(() => useGraphFilterState());

    act(() => {
      result.current.syncWithGraph(initialGraph, true);
      result.current.toggleChain("BTC", false);
      result.current.updateDate("startTime", "2026-01-02T00:00:00Z");
      result.current.updateNumber("maxTxnUSD", "5");
    });
    act(() => {
      result.current.syncWithGraph(rebuiltGraph, true);
    });

    expect(result.current.filtersActive).toBe(false);
    expect(result.current.graphFilters.selectedChains).toEqual(["ETH"]);
    expect(result.current.graphFilters.startTime).toBe("2026-02-01T00:00:00.000Z");
    expect(result.current.graphFilters.maxTxnUSD).toBe(90);

    act(() => {
      result.current.toggleTxnType("swap", false);
      result.current.resetAllFilters();
    });

    expect(result.current.filtersActive).toBe(false);
    expect(result.current.graphFilters.txnTypes.swap).toBe(true);
    expect(result.current.graphFilters.startTime).toBe(result.current.graphFilters.graphMinTime);
  });
});
