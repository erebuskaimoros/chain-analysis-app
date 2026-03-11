import { describe, expect, it } from "vitest";
import {
  applyNodeUpdates,
  mergeActorGraphResponse,
  mergeAddressExplorerResponse,
  mergeExplorerExpansionResponse,
} from "../../graph";
import {
  makeActor,
  makeActorGraphResponse,
  makeEdge,
  makeExplorerResponse,
  makeNode,
  makeSupportingAction,
  makeTransaction,
} from "../../../test-support/graphFixtures";

describe("graph merges", () => {
  it("merges actor responses by canonical node identity and aggregates duplicate edges", () => {
    const current = makeActorGraphResponse({
      actors: [makeActor({ id: 1, name: "Alpha" })],
      warnings: ["current warning"],
      nodes: [
        makeNode({
          id: "actor-node-a",
          kind: "actor_address",
          chain: "THOR",
          actor_ids: [1],
          metrics: { address: "thor1shared" },
        }),
        makeNode({ id: "target-node", chain: "BTC", metrics: { address: "bc1target" } }),
      ],
      edges: [
        makeEdge({
          id: "edge-a",
          from: "actor-node-a",
          to: "target-node",
          actor_ids: [1],
          transactions: [makeTransaction({ tx_id: "tx-a", usd_spot: 10 })],
        }),
      ],
      supporting_actions: [makeSupportingAction({ tx_id: "support-a", from_node: "actor-node-a", to_node: "target-node" })],
    });
    const incoming = makeActorGraphResponse({
      actors: [makeActor({ id: 2, name: "Beta" })],
      warnings: ["incoming warning"],
      nodes: [
        makeNode({
          id: "actor-node-b",
          kind: "actor_address",
          chain: "THOR",
          actor_ids: [2],
          metrics: { address: "thor1shared" },
        }),
        makeNode({ id: "fresh-node", chain: "ETH", metrics: { address: "0xfresh" } }),
        makeNode({ id: "target-node", chain: "BTC", metrics: { address: "bc1target" } }),
      ],
      edges: [
        makeEdge({
          id: "edge-b",
          from: "actor-node-b",
          to: "target-node",
          actor_ids: [2],
          transactions: [makeTransaction({ tx_id: "tx-b", usd_spot: 20 })],
        }),
      ],
      supporting_actions: [makeSupportingAction({ tx_id: "support-b", from_node: "actor-node-b", to_node: "target-node" })],
    });

    const merged = mergeActorGraphResponse(current, incoming);

    expect(merged.actors.map((actor) => actor.id).sort()).toEqual([1, 2]);
    expect(merged.nodes).toHaveLength(3);
    expect(merged.nodes.find((node) => node.id === "actor-node-a")?.actor_ids.sort()).toEqual([1, 2]);
    expect(merged.edges).toHaveLength(1);
    expect(merged.edges[0]?.tx_ids).toEqual(["tx-a", "tx-b"]);
    expect(merged.edges[0]?.usd_spot).toBe(30);
    expect(merged.supporting_actions.map((action) => action.tx_id).sort()).toEqual(["support-a", "support-b"]);
    expect(merged.warnings.sort()).toEqual(["current warning", "incoming warning"]);
    expect(merged.stats.actor_count).toBe(2);
    expect(merged.stats.edge_count).toBe(1);
  });

  it("merges explorer pagination responses while preserving explorer metadata", () => {
    const current = makeExplorerResponse({
      address: "thor1base",
      raw_address: "thor1base",
      loaded_actions: 10,
      has_more: true,
      next_offset: 10,
      warnings: ["current warning"],
      nodes: [makeNode({ id: "base-node", chain: "THOR", metrics: { address: "thor1base" } })],
      supporting_actions: [makeSupportingAction({ tx_id: "support-a", from_node: "base-node", to_node: "base-node" })],
    });
    const incoming = makeExplorerResponse({
      address: "thor1base",
      raw_address: "thor1base",
      loaded_actions: 5,
      has_more: false,
      next_offset: 15,
      warnings: ["incoming warning"],
      nodes: [makeNode({ id: "new-node", chain: "BTC", metrics: { address: "bc1new" } })],
      supporting_actions: [makeSupportingAction({ tx_id: "support-b", from_node: "new-node", to_node: "base-node" })],
    });

    const merged = mergeAddressExplorerResponse(current, incoming);

    expect(merged.address).toBe("thor1base");
    expect(merged.loaded_actions).toBe(15);
    expect(merged.has_more).toBe(false);
    expect(merged.next_offset).toBe(15);
    expect(merged.nodes.map((node) => node.id).sort()).toEqual(["base-node", "new-node"]);
    expect(merged.warnings.sort()).toEqual(["current warning", "incoming warning"]);
  });

  it("requires a base explorer response for expansion payloads and preserves existing explorer state", () => {
    const expansion = makeActorGraphResponse({
      nodes: [makeNode({ id: "expanded-node", chain: "BTC", metrics: { address: "bc1expanded" } })],
      edges: [
        makeEdge({
          id: "expanded-edge",
          from: "expanded-node",
          to: "expanded-node",
          transactions: [makeTransaction({ tx_id: "expanded-tx", usd_spot: 5 })],
        }),
      ],
      warnings: ["expansion warning"],
    });

    expect(() => mergeExplorerExpansionResponse(null, expansion)).toThrow(
      "Explorer merge requires a base response for expansion payloads."
    );

    const base = makeExplorerResponse({
      address: "thor1base",
      raw_address: "thor1base",
      mode: "graph",
      loaded_actions: 7,
      has_more: true,
      warnings: ["base warning"],
      nodes: [makeNode({ id: "base-node", chain: "THOR", metrics: { address: "thor1base" } })],
    });

    const merged = mergeExplorerExpansionResponse(base, expansion);

    expect(merged.address).toBe("thor1base");
    expect(merged.raw_address).toBe("thor1base");
    expect(merged.mode).toBe("graph");
    expect(merged.loaded_actions).toBe(7);
    expect(merged.has_more).toBe(true);
    expect(merged.nodes.map((node) => node.id).sort()).toEqual(["base-node", "expanded-node"]);
    expect(merged.warnings.sort()).toEqual(["base warning", "expansion warning"]);
  });

  it("reconciles expansion nodes onto the existing explorer target when the address matches", () => {
    const base = makeExplorerResponse({
      address: "thor1base",
      raw_address: "thor1base",
      nodes: [
        makeNode({
          id: "explorer-target",
          kind: "explorer_target",
          chain: "THOR",
          stage: "protocol",
          label: "thor1base",
          metrics: { address: "thor1base", out_edges: 2 },
        }),
      ],
    });
    const expansion = makeActorGraphResponse({
      nodes: [
        makeNode({
          id: "expanded-same-address",
          kind: "external_address",
          chain: "THOR",
          stage: "external",
          label: "thor1base",
          metrics: { address: "thor1base", in_edges: 1 },
        }),
        makeNode({
          id: "expanded-peer",
          kind: "external_address",
          chain: "BTC",
          metrics: { address: "bc1peer" },
        }),
      ],
      edges: [
        makeEdge({
          id: "expanded-edge",
          from: "expanded-same-address",
          to: "expanded-peer",
          transactions: [makeTransaction({ tx_id: "expanded-tx", usd_spot: 5 })],
        }),
      ],
    });

    const merged = mergeExplorerExpansionResponse(base, expansion);
    const baseAddressHits = merged.nodes.filter((node) => String(node.metrics?.address || "") === "thor1base");

    expect(baseAddressHits).toHaveLength(1);
    expect(baseAddressHits[0]?.id).toBe("explorer-target");
    expect(baseAddressHits[0]?.kind).toBe("explorer_target");
    expect(baseAddressHits[0]?.metrics).toMatchObject({
      address: "thor1base",
      out_edges: 2,
      in_edges: 1,
    });
    expect(merged.edges[0]?.from).toBe("explorer-target");
  });

  it("dedupes explorer expansion supporting actions when matching nodes arrive with new raw IDs", () => {
    const base = makeExplorerResponse({
      address: "thor1base",
      raw_address: "thor1base",
      nodes: [
        makeNode({
          id: "base-source",
          kind: "external_address",
          chain: "THOR",
          metrics: { address: "thor1same-source" },
        }),
        makeNode({
          id: "base-target",
          kind: "external_address",
          chain: "BTC",
          metrics: { address: "bc1same-target" },
        }),
      ],
      supporting_actions: [
        makeSupportingAction({
          tx_id: "dup-tx",
          from_node: "base-source",
          to_node: "base-target",
        }),
      ],
    });
    const expansion = makeActorGraphResponse({
      nodes: [
        makeNode({
          id: "expanded-source-id",
          kind: "external_address",
          chain: "THOR",
          metrics: { address: "thor1same-source" },
        }),
        makeNode({
          id: "expanded-target-id",
          kind: "external_address",
          chain: "BTC",
          metrics: { address: "bc1same-target" },
        }),
      ],
      supporting_actions: [
        makeSupportingAction({
          tx_id: "dup-tx",
          from_node: "expanded-source-id",
          to_node: "expanded-target-id",
        }),
      ],
    });

    const merged = mergeExplorerExpansionResponse(base, expansion);

    expect(merged.supporting_actions).toHaveLength(1);
    expect(merged.supporting_actions[0]?.from_node).toBe("base-source");
    expect(merged.supporting_actions[0]?.to_node).toBe("base-target");
    expect(merged.stats.supporting_action_count).toBe(1);
  });

  it("keeps supporting actions distinct when the source protocol differs", () => {
    const current = makeActorGraphResponse({
      supporting_actions: [
        makeSupportingAction({
          tx_id: "dup-tx",
          source_protocol: "THOR",
          from_node: "node-a",
          to_node: "node-b",
        }),
      ],
    });
    const incoming = makeActorGraphResponse({
      supporting_actions: [
        makeSupportingAction({
          tx_id: "dup-tx",
          source_protocol: "MAYA",
          from_node: "node-a",
          to_node: "node-b",
        }),
      ],
    });

    const merged = mergeActorGraphResponse(current, incoming);

    expect(merged.supporting_actions).toHaveLength(2);
    expect(merged.supporting_actions.map((action) => action.source_protocol).sort()).toEqual(["MAYA", "THOR"]);
  });

  it("keeps duplicate transaction ids separate when they come from different liquidity engines", () => {
    const current = makeActorGraphResponse({
      nodes: [
        makeNode({ id: "node-a", metrics: { address: "thor1same" } }),
        makeNode({ id: "node-b", metrics: { address: "bc1same" } }),
      ],
      edges: [
        makeEdge({
          id: "edge-thor",
          from: "node-a",
          to: "node-b",
          source_protocols: ["THOR"],
          transactions: [makeTransaction({ tx_id: "same-tx", source_protocol: "THOR", usd_spot: 10 })],
        }),
      ],
    });
    const incoming = makeActorGraphResponse({
      nodes: [
        makeNode({ id: "node-a", metrics: { address: "thor1same" } }),
        makeNode({ id: "node-b", metrics: { address: "bc1same" } }),
      ],
      edges: [
        makeEdge({
          id: "edge-maya",
          from: "node-a",
          to: "node-b",
          source_protocols: ["MAYA"],
          transactions: [makeTransaction({ tx_id: "same-tx", source_protocol: "MAYA", usd_spot: 20 })],
        }),
      ],
    });

    const merged = mergeActorGraphResponse(current, incoming);

    expect(merged.edges).toHaveLength(1);
    expect(merged.edges[0]?.transactions).toHaveLength(2);
    expect(merged.edges[0]?.source_protocols?.sort()).toEqual(["MAYA", "THOR"]);
    expect(merged.edges[0]?.transactions.map((tx) => tx.source_protocol).sort()).toEqual(["MAYA", "THOR"]);
  });

  it("applies node updates by merging metrics onto the existing node list", () => {
    const currentNodes = [
      makeNode({
        id: "node-a",
        metrics: {
          address: "thor1a",
          live_holdings_status: "error",
          stale: true,
        },
      }),
      makeNode({ id: "node-b", metrics: { address: "thor1b", untouched: true } }),
    ];
    const updates = [
      makeNode({
        id: "node-a",
        metrics: {
          live_holdings_status: "available",
          live_holdings_available: true,
          live_holdings_usd_spot: 99,
        },
      }),
    ];

    const nextNodes = applyNodeUpdates(currentNodes, updates);

    expect(nextNodes[0]?.metrics).toEqual({
      address: "thor1a",
      live_holdings_status: "available",
      stale: true,
      live_holdings_available: true,
      live_holdings_usd_spot: 99,
    });
    expect(nextNodes[1]).toEqual(currentNodes[1]);
  });
});
