import { describe, expect, it } from "vitest";
import {
  refreshableLiveValueNodes,
  unavailableRawNodes,
} from "../../graph";
import {
  makeExplorerResponse,
  makeNode,
} from "../../../test-support/graphFixtures";

describe("graph live-value helpers", () => {
  it("filters pool and validator nodes out of refresh payloads", () => {
    const nodes = [
      makeNode({
        id: "pool-node",
        kind: "pool",
        metrics: { pool: "THOR.RUNE", live_holdings_status: "available" },
      }),
      makeNode({
        id: "validator-node",
        kind: "node",
        metrics: { address: "thor1validator", live_holdings_status: "available" },
      }),
      makeNode({
        id: "wallet-node",
        kind: "external_address",
        metrics: { address: "0xwatch", live_holdings_status: "error" },
      }),
    ];

    expect(refreshableLiveValueNodes(nodes).map((node) => node.id)).toEqual(["wallet-node"]);
  });

  it("includes pool and validator nodes with failed live values in refresh payloads", () => {
    const nodes = [
      makeNode({
        id: "pool-ok",
        kind: "pool",
        metrics: { pool: "THOR.RUNE", live_holdings_status: "available" },
      }),
      makeNode({
        id: "pool-failed",
        kind: "pool",
        metrics: { pool: "ETH.ETH", live_holdings_status: "error" },
      }),
      makeNode({
        id: "pool-missing",
        kind: "pool",
        metrics: { pool: "BTC.BTC" },
      }),
      makeNode({
        id: "validator-ok",
        kind: "node",
        metrics: { address: "thor1ok", live_holdings_status: "available" },
      }),
      makeNode({
        id: "validator-failed",
        kind: "node",
        metrics: { address: "thor1fail", live_holdings_status: "error" },
      }),
      makeNode({
        id: "wallet-node",
        kind: "external_address",
        metrics: { address: "0xwatch" },
      }),
    ];

    expect(refreshableLiveValueNodes(nodes).map((node) => node.id)).toEqual([
      "pool-failed",
      "pool-missing",
      "validator-failed",
      "wallet-node",
    ]);
  });

  it("excludes actor containers and already-available address nodes from refresh payloads", () => {
    const nodes = [
      makeNode({
        id: "actor-node",
        kind: "actor",
        metrics: {},
      }),
      makeNode({
        id: "actor-address-ok",
        kind: "actor_address",
        metrics: { address: "thor1actor", live_holdings_status: "available" },
      }),
      makeNode({
        id: "external-ok",
        kind: "external_address",
        metrics: { address: "0xready", live_holdings_status: "available" },
      }),
      makeNode({
        id: "known-error",
        kind: "known",
        metrics: { address: "thor1known", live_holdings_status: "error" },
      }),
      makeNode({
        id: "router-missing",
        kind: "router",
        metrics: { address: "0xrouter" },
      }),
    ];

    expect(refreshableLiveValueNodes(nodes).map((node) => node.id)).toEqual(["known-error", "router-missing"]);
  });

  it("includes failed pool and validator nodes in unavailable refresh candidates", () => {
    const graph = makeExplorerResponse({
      nodes: [
        makeNode({
          id: "pool-failed",
          kind: "pool",
          metrics: { pool: "ETH.ETH", live_holdings_status: "error" },
        }),
        makeNode({
          id: "validator-failed",
          kind: "node",
          metrics: { address: "thor1fail", live_holdings_status: "unavailable" },
        }),
        makeNode({
          id: "wallet-node",
          kind: "external_address",
          metrics: { address: "0xwatch", live_holdings_status: "error" },
        }),
        makeNode({
          id: "healthy-wallet",
          kind: "external_address",
          metrics: { address: "bc1watch", live_holdings_status: "available" },
        }),
      ],
    });

    expect(unavailableRawNodes(graph).map((node) => node.id)).toEqual([
      "pool-failed",
      "validator-failed",
      "wallet-node",
    ]);
  });

  it("excludes successfully-computed inline nodes from unavailable refresh candidates", () => {
    const graph = makeExplorerResponse({
      nodes: [
        makeNode({
          id: "pool-ok",
          kind: "pool",
          metrics: { pool: "THOR.RUNE", live_holdings_status: "available" },
        }),
        makeNode({
          id: "validator-ok",
          kind: "node",
          metrics: { address: "thor1validator", live_holdings_status: "available" },
        }),
        makeNode({
          id: "wallet-node",
          kind: "external_address",
          metrics: { address: "0xwatch", live_holdings_status: "error" },
        }),
        makeNode({
          id: "healthy-wallet",
          kind: "external_address",
          metrics: { address: "bc1watch", live_holdings_status: "available" },
        }),
      ],
    });

    expect(unavailableRawNodes(graph).map((node) => node.id)).toEqual(["wallet-node"]);
  });
});
