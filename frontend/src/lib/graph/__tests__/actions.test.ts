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

  it("excludes inline-computed nodes from unavailable refresh candidates", () => {
    const graph = makeExplorerResponse({
      nodes: [
        makeNode({
          id: "pool-node",
          kind: "pool",
          metrics: { pool: "THOR.RUNE", live_holdings_status: "error" },
        }),
        makeNode({
          id: "validator-node",
          kind: "node",
          metrics: { address: "thor1validator", live_holdings_status: "unavailable" },
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
