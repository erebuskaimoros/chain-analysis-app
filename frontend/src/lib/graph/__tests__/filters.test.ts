import { describe, expect, it } from "vitest";
import {
  createGraphFilterState,
  filterSupportingActions,
  graphFiltersAreActive,
  resetGraphFilters,
  setGraphFilterDateValue,
  setGraphFilterNumberValue,
  syncGraphFilterStateWithResponse,
} from "../../graph";
import {
  makeEdge,
  makeExplorerResponse,
  makeNode,
  makeSupportingAction,
  makeTransaction,
} from "../../../test-support/graphFixtures";

describe("graph filters", () => {
  it("syncs metadata, clamps stale filter values, and resets to the current graph range", () => {
    const initialResponse = makeExplorerResponse({
      nodes: [
        makeNode({ id: "btc-source", chain: "BTC", metrics: { address: "bc1source" } }),
        makeNode({ id: "thor-target", chain: "THOR", metrics: { address: "thor1target" } }),
      ],
      edges: [
        makeEdge({
          id: "edge-a",
          from: "btc-source",
          to: "thor-target",
          transactions: [
            makeTransaction({ tx_id: "tx-a", time: "2026-01-01T00:00:00Z", usd_spot: 10 }),
            makeTransaction({ tx_id: "tx-b", time: "2026-01-05T00:00:00Z", usd_spot: 50 }),
          ],
        }),
      ],
    });
    const replacementResponse = makeExplorerResponse({
      nodes: [
        makeNode({ id: "thor-left", chain: "THOR", metrics: { address: "thor1left" } }),
        makeNode({ id: "thor-right", chain: "THOR", metrics: { address: "thor1right" } }),
      ],
      edges: [
        makeEdge({
          id: "edge-b",
          from: "thor-left",
          to: "thor-right",
          transactions: [makeTransaction({ tx_id: "tx-c", time: "2026-01-03T00:00:00Z", usd_spot: 20 })],
        }),
      ],
    });

    const filterState = createGraphFilterState();
    syncGraphFilterStateWithResponse(filterState, initialResponse, { reset: true });

    expect(filterState.availableChains).toEqual(["BTC", "THOR"]);
    expect(filterState.graphMinTime).toBe("2026-01-01T00:00:00.000Z");
    expect(filterState.graphMaxTime).toBe("2026-01-05T00:00:00.000Z");
    expect(filterState.graphMinTxnUSD).toBe(10);
    expect(filterState.graphMaxTxnUSD).toBe(50);

    filterState.selectedChains = ["BTC"];
    setGraphFilterDateValue(filterState, "startTime", "2026-01-04T00:00:00Z");
    setGraphFilterNumberValue(filterState, "maxTxnUSD", "12");

    syncGraphFilterStateWithResponse(filterState, replacementResponse, { reset: false });

    expect(filterState.selectedChains).toEqual(["THOR"]);
    expect(filterState.startTime).toBe("2026-01-03T00:00:00.000Z");
    expect(filterState.endTime).toBe("2026-01-03T00:00:00.000Z");
    expect(filterState.minTxnUSD).toBe(20);
    expect(filterState.maxTxnUSD).toBe(20);

    filterState.txnTypes.swap = false;
    expect(graphFiltersAreActive(filterState)).toBe(true);

    resetGraphFilters(filterState);

    expect(graphFiltersAreActive(filterState)).toBe(false);
    expect(filterState.selectedChains).toEqual(["THOR"]);
    expect(filterState.startTime).toBe(filterState.graphMinTime);
    expect(filterState.endTime).toBe(filterState.graphMaxTime);
    expect(filterState.minTxnUSD).toBe(filterState.graphMinTxnUSD);
    expect(filterState.maxTxnUSD).toBe(filterState.graphMaxTxnUSD);
  });

  it("reports inactive filters only when every graph dimension matches the full graph", () => {
    const response = makeExplorerResponse({
      nodes: [
        makeNode({ id: "thor-node", chain: "THOR", metrics: { address: "thor1node" } }),
        makeNode({ id: "btc-node", chain: "BTC", metrics: { address: "bc1node" } }),
      ],
      edges: [makeEdge({ from: "thor-node", to: "btc-node" })],
    });
    const filterState = createGraphFilterState();

    syncGraphFilterStateWithResponse(filterState, response, { reset: true });
    expect(graphFiltersAreActive(filterState)).toBe(false);

    filterState.txnTypes.swap = false;
    expect(graphFiltersAreActive(filterState)).toBe(true);
  });

  it("filters supporting actions by txn bucket, chain, time, and value", () => {
    const response = makeExplorerResponse({
      nodes: [
        makeNode({ id: "thor-node", chain: "THOR", metrics: { address: "thor1node" } }),
        makeNode({ id: "btc-node", chain: "BTC", metrics: { address: "bc1node" } }),
        makeNode({ id: "eth-node", chain: "ETH", metrics: { address: "0xnode" } }),
      ],
      edges: [
        makeEdge({
          id: "edge-transfer",
          from: "thor-node",
          to: "btc-node",
          action_class: "transfers",
          action_key: "transfer",
          action_label: "Transfer",
          transactions: [makeTransaction({ tx_id: "transfer-edge-tx", time: "2026-01-02T10:00:00Z", usd_spot: 25 })],
        }),
        makeEdge({
          id: "edge-swap",
          from: "btc-node",
          to: "eth-node",
          action_class: "swaps",
          action_key: "swap",
          action_label: "Swap",
          action_domain: "swap",
          transactions: [makeTransaction({ tx_id: "swap-edge-tx", time: "2026-01-02T12:00:00Z", usd_spot: 75 })],
        }),
      ],
    });
    const actions = [
      makeSupportingAction({
        tx_id: "transfer-action",
        action_class: "transfers",
        action_key: "transfer",
        action_label: "Transfer",
        from_node: "thor-node",
        to_node: "btc-node",
        time: "2026-01-02T10:00:00Z",
        usd_spot: 25,
      }),
      makeSupportingAction({
        tx_id: "swap-action",
        action_class: "swaps",
        action_key: "swap",
        action_label: "Swap",
        action_domain: "swap",
        from_node: "btc-node",
        to_node: "eth-node",
        time: "2026-01-02T12:00:00Z",
        usd_spot: 75,
      }),
      makeSupportingAction({
        tx_id: "late-transfer",
        action_class: "transfers",
        action_key: "transfer",
        action_label: "Transfer",
        from_node: "eth-node",
        to_node: "btc-node",
        time: "2026-01-04T12:00:00Z",
        usd_spot: 25,
      }),
    ];
    const filterState = createGraphFilterState();

    syncGraphFilterStateWithResponse(filterState, response, { reset: true });
    filterState.txnTypes.swap = false;
    filterState.selectedChains = ["BTC"];
    filterState.startTime = "2026-01-02T00:00:00.000Z";
    filterState.endTime = "2026-01-02T23:59:59.000Z";
    filterState.maxTxnUSD = 40;

    const filtered = filterSupportingActions(actions, response, filterState);

    expect(filtered.map((action) => action.tx_id)).toEqual(["transfer-action"]);
  });
});
