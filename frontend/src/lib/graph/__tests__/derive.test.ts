import { describe, expect, it } from "vitest";
import {
  createGraphFilterState,
  deriveActorVisibleGraph,
  deriveExplorerVisibleGraph,
  syncGraphFilterStateWithResponse,
} from "../../graph";
import {
  makeActor,
  makeActorAddress,
  makeActorGraphResponse,
  makeAnnotation,
  makeBlocklistedAddress,
  makeEdge,
  makeExplorerResponse,
  makeMetadata,
  makeNode,
  makeTransaction,
} from "../../../test-support/graphFixtures";

describe("graph derivation", () => {
  it("removes blocklisted and Asgard-labelled addresses from actor graphs", () => {
    const response = makeActorGraphResponse({
      nodes: [
        makeNode({ id: "visible-a", chain: "THOR", metrics: { address: "thor1visiblea" } }),
        makeNode({ id: "visible-b", chain: "BTC", metrics: { address: "bc1visibleb" } }),
        makeNode({ id: "blocked-node", chain: "ETH", metrics: { address: "0xblocked" } }),
        makeNode({ id: "asgard-node", chain: "THOR", metrics: { address: "thor1asgard" } }),
      ],
      edges: [
        makeEdge({
          id: "visible-edge",
          from: "visible-a",
          to: "visible-b",
          transactions: [makeTransaction({ tx_id: "visible-tx", usd_spot: 15 })],
        }),
        makeEdge({
          id: "blocked-edge",
          from: "blocked-node",
          to: "visible-b",
          transactions: [makeTransaction({ tx_id: "blocked-tx", usd_spot: 20 })],
        }),
        makeEdge({
          id: "asgard-edge",
          from: "visible-a",
          to: "asgard-node",
          transactions: [makeTransaction({ tx_id: "asgard-tx", usd_spot: 25 })],
        }),
      ],
    });
    const filters = createGraphFilterState();
    const metadata = makeMetadata({
      blocklist: [makeBlocklistedAddress({ normalized_address: "0xblocked" })],
      annotations: [makeAnnotation({ kind: "asgard_vault", value: "true", normalized_address: "thor1asgard" })],
    });

    syncGraphFilterStateWithResponse(filters, response, { reset: true });
    const visible = deriveActorVisibleGraph(response, filters, metadata, {
      expandedActorIDs: [],
      expandedExternalChains: [],
    });

    expect(visible.nodes.map((node) => node.id).sort()).toEqual(["visible-a", "visible-b"]);
    expect(visible.edges).toHaveLength(1);
    expect(visible.edges[0]?.raw_edge_ids).toEqual(["visible-edge"]);
  });

  it("preserves collapsed actor-address and low-signal external-cluster behavior", () => {
    const response = makeActorGraphResponse({
      query: {
        collapse_external: true,
        min_usd: 100,
      },
      actors: [
        makeActor({
          id: 1,
          name: "Alice",
          color: "#225588",
          addresses: [makeActorAddress({ actor_id: 1, address: "thor1alice", normalized_address: "thor1alice" })],
        }),
      ],
      nodes: [
        makeNode({
          id: "actor-address",
          kind: "actor_address",
          chain: "THOR",
          actor_ids: [1],
          label: "Alice Address",
          metrics: { address: "thor1alice" },
        }),
        makeNode({
          id: "external-address",
          kind: "external_address",
          chain: "BTC",
          label: "BTC External",
          metrics: { address: "bc1external" },
        }),
      ],
      edges: [
        makeEdge({
          id: "low-signal-edge",
          from: "actor-address",
          to: "external-address",
          transactions: [makeTransaction({ tx_id: "clustered-tx", usd_spot: 50 })],
        }),
      ],
    });
    const filters = createGraphFilterState();

    syncGraphFilterStateWithResponse(filters, response, { reset: true });
    const visible = deriveActorVisibleGraph(response, filters, makeMetadata(), {
      expandedActorIDs: [],
      expandedExternalChains: [],
    });

    const actorNode = visible.nodes.find((node) => node.id === "actor:1");
    const externalCluster = visible.nodes.find((node) => node.id === "external_cluster:BTC");

    expect(actorNode?.label).toBe("Alice");
    expect(actorNode?.raw_node_ids).toEqual(["actor-address"]);
    expect(externalCluster?.kind).toBe("external_cluster");
    expect(externalCluster?.raw_node_ids).toEqual(["external-address"]);
    expect(visible.edges).toHaveLength(1);
    expect(visible.edges[0]?.source).toBe("actor:1");
    expect(visible.edges[0]?.target).toBe("external_cluster:BTC");
  });

  it("prunes filtered explorer transactions, edges, and nodes", () => {
    const response = makeExplorerResponse({
      nodes: [
        makeNode({ id: "explorer-a", chain: "THOR", metrics: { address: "thor1a" } }),
        makeNode({ id: "explorer-b", chain: "BTC", metrics: { address: "bc1b" } }),
        makeNode({ id: "explorer-c", chain: "ETH", metrics: { address: "0xc" } }),
      ],
      edges: [
        makeEdge({
          id: "mixed-transfer-edge",
          from: "explorer-a",
          to: "explorer-b",
          action_class: "transfers",
          action_key: "transfer",
          action_label: "Transfer",
          transactions: [
            makeTransaction({ tx_id: "tx-in-range", time: "2026-01-01T00:00:00Z", usd_spot: 25 }),
            makeTransaction({ tx_id: "tx-out-of-range", time: "2026-01-05T00:00:00Z", usd_spot: 200 }),
          ],
        }),
        makeEdge({
          id: "swap-edge",
          from: "explorer-b",
          to: "explorer-c",
          action_class: "swaps",
          action_key: "swap",
          action_label: "Swap",
          action_domain: "swap",
          transactions: [makeTransaction({ tx_id: "swap-tx", time: "2026-01-04T00:00:00Z", usd_spot: 75 })],
        }),
      ],
    });
    const filters = createGraphFilterState();

    syncGraphFilterStateWithResponse(filters, response, { reset: true });
    filters.endTime = "2026-01-02T00:00:00.000Z";
    filters.maxTxnUSD = 50;
    filters.txnTypes.swap = false;

    const visible = deriveExplorerVisibleGraph(response, filters, makeMetadata());

    expect(visible.nodes.map((node) => node.id).sort()).toEqual(["explorer-a", "explorer-b"]);
    expect(visible.edges).toHaveLength(1);
    expect(visible.edges[0]?.transactions.map((transaction) => transaction.tx_id)).toEqual(["tx-in-range"]);
  });

  it("keeps explorer targets styled like address nodes instead of validator nodes", () => {
    const response = makeExplorerResponse({
      nodes: [
        makeNode({
          id: "seed",
          kind: "explorer_target",
          label: "thor1seed",
          chain: "THOR",
          metrics: { address: "thor1seed", live_holdings_available: false, live_holdings_status: "" },
        }),
        makeNode({
          id: "validator",
          kind: "node",
          label: "Node thor1validator",
          chain: "THOR",
          metrics: { address: "thor1validator", node_total_bond: "100000000" },
        }),
      ],
      edges: [
        makeEdge({
          id: "bond-edge",
          from: "seed",
          to: "validator",
          action_class: "bonds",
          action_key: "bond",
          action_label: "Bond",
          transactions: [makeTransaction({ tx_id: "bond-tx", usd_spot: 25 })],
        }),
      ],
    });
    const filters = createGraphFilterState();

    syncGraphFilterStateWithResponse(filters, response, { reset: true });
    const visible = deriveExplorerVisibleGraph(response, filters, makeMetadata());

    const seed = visible.nodes.find((node) => node.id === "seed");
    const validator = visible.nodes.find((node) => node.id === "validator");

    expect(seed?.kind).toBe("explorer_target");
    expect(seed?.color).toBe("#5f86be");
    expect(seed?.borderColor).toBe("#f5c76e");
    expect(validator?.kind).toBe("node");
    expect(validator?.color).toBe("#c86b1f");
  });

  it("propagates edge USD totals onto visible node metrics", () => {
    const response = makeActorGraphResponse({
      nodes: [
        makeNode({ id: "ruji-source", chain: "THOR", metrics: { address: "thor1source" } }),
        makeNode({ id: "ruji-target", chain: "THOR", metrics: { address: "thor1target" } }),
      ],
      edges: [
        makeEdge({
          id: "ruji-edge",
          from: "ruji-source",
          to: "ruji-target",
          action_class: "transfers",
          action_key: "transfer",
          action_label: "Transfer",
          assets: [
            {
              asset: "THOR.RUJI",
              amount_raw: "800000000",
              usd_spot: 2,
              asset_kind: "native",
            },
          ],
          transactions: [makeTransaction({ tx_id: "ruji-tx", usd_spot: 2 })],
          usd_spot: 2,
        }),
      ],
    });
    const filters = createGraphFilterState();

    syncGraphFilterStateWithResponse(filters, response, { reset: true });
    const visible = deriveActorVisibleGraph(response, filters, makeMetadata(), {
      expandedActorIDs: [],
      expandedExternalChains: [],
    });

    const source = visible.nodes.find((node) => node.id === "ruji-source");
    const target = visible.nodes.find((node) => node.id === "ruji-target");

    expect(source?.metrics?.usd_spot).toBe(2);
    expect(target?.metrics?.usd_spot).toBe(2);
  });
});
