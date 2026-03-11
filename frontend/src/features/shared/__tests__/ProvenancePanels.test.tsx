import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ActionLookupPanel } from "../ActionLookupPanel";
import { SelectionInspector } from "../SelectionInspector";
import { SupportingActionsTable } from "../SupportingActionsTable";
import { makeSupportingAction, makeVisibleNode } from "../../../test-support/graphFixtures";
import type { VisibleGraphEdge } from "../../../lib/graph";

describe("provenance panels", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders source protocol rows in the action lookup panel", () => {
    render(
      <ActionLookupPanel
        result={{
          tx_id: "tx-1",
          actions: [{ source_protocol: "MAYA", type: "swap", height: "123", status: "success" }],
        }}
        isLoading={false}
        error=""
      />
    );

    expect(screen.getByText("Source")).toBeTruthy();
    expect(screen.getByText("MAYA")).toBeTruthy();
    expect(screen.getByText("swap")).toBeTruthy();
  });

  it("renders supporting action provenance in the table", () => {
    render(
      <SupportingActionsTable
        actions={[
          makeSupportingAction({
            tx_id: "support-1",
            source_protocol: "THOR",
          }),
        ]}
        onLookup={vi.fn()}
      />
    );

    expect(screen.getByText("Source")).toBeTruthy();
    expect(screen.getByText("THOR")).toBeTruthy();
  });

  it("shows source protocols for node and edge inspection", () => {
    const node = makeVisibleNode({
      metrics: {
        address: "thor1node",
        source_protocols: ["THOR", "MAYA"],
      },
    });
    const edge: VisibleGraphEdge = {
      id: "edge-1",
      from: "node-a",
      to: "node-b",
      source: "node-a",
      target: "node-b",
      action_class: "transfers",
      action_key: "transfer",
      action_label: "Transfer",
      action_domain: "transfer",
      assets: [],
      transactions: [
        { tx_id: "tx-1", height: 1, time: "2026-01-01T00:00:00Z", usd_spot: 10, assets: [], source_protocol: "THOR" },
        { tx_id: "tx-1", height: 1, time: "2026-01-01T00:00:00Z", usd_spot: 20, assets: [], source_protocol: "MAYA" },
      ],
      usd_spot: 30,
      tx_ids: ["tx-1"],
      heights: [1],
      actor_ids: [],
      confidence: 1,
      source_protocols: ["THOR", "MAYA"],
      width: 2,
      lineColor: "#fff",
      edgeLabel: "Transfer",
      action_classes: ["transfers"],
      action_keys: ["transfer"],
      action_labels: ["Transfer"],
      action_domains: ["transfer"],
      action_buckets: ["transfer"],
      validator_addresses: [],
      validator_labels: [],
      contract_types: [],
      contract_protocols: [],
      chain_set: ["THOR"],
      raw_edge_ids: ["edge-1"],
      inspect: { source_protocols: ["THOR", "MAYA"] },
    };

    const { rerender } = render(
      <SelectionInspector selection={{ kind: "node", node }} emptyMessage="Nothing selected" />
    );

    expect(screen.getByText("Source Protocols")).toBeTruthy();
    expect(screen.getByText("THOR, MAYA")).toBeTruthy();

    rerender(
      <SelectionInspector
        selection={{ kind: "edge", edge }}
        emptyMessage="Nothing selected"
        onLookupTx={vi.fn()}
      />
    );

    expect(screen.getAllByText("THOR").length).toBeGreaterThan(0);
    expect(screen.getAllByText("MAYA").length).toBeGreaterThan(0);
  });
});
