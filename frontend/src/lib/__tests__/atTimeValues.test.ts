import { describe, expect, it } from "vitest";
import { preferAtTimeValues } from "../atTimeValues";
import type { FlowEdge, SupportingAction } from "../types";

describe("preferAtTimeValues", () => {
  it("values edges, transactions, assets, and actions at transaction time", () => {
    const edge = {
      id: "e",
      usd_spot: 500,
      usd_at_time: 120,
      assets: [{ asset: "ETH.ETH", amount_raw: "1", usd_spot: 500, usd_at_time: 120 }],
      transactions: [
        { tx_id: "T", height: 1, time: "", usd_spot: 500, usd_at_time: 120, assets: [{ asset: "ETH.ETH", amount_raw: "1", usd_spot: 500, usd_at_time: 120 }] },
      ],
    } as unknown as FlowEdge;
    const action = { tx_id: "T", usd_spot: 500, usd_at_time: 120 } as unknown as SupportingAction;

    const out = preferAtTimeValues({ edges: [edge], supporting_actions: [action] });
    expect(out.edges?.[0].usd_spot).toBe(120);
    expect(out.edges?.[0].usd_now).toBe(500);
    expect(out.edges?.[0].assets[0].usd_spot).toBe(120);
    expect(out.edges?.[0].transactions[0].usd_spot).toBe(120);
    expect(out.edges?.[0].transactions[0].assets[0].usd_now).toBe(500);
    expect(out.supporting_actions?.[0].usd_spot).toBe(120);
  });

  it("leaves graphs without at-time values unchanged and is idempotent", () => {
    const legacy = { edges: [{ id: "e", usd_spot: 50, assets: [], transactions: [] } as unknown as FlowEdge] };
    expect(preferAtTimeValues(legacy).edges?.[0].usd_spot).toBe(50);
    const once = preferAtTimeValues({ edges: [{ id: "e", usd_spot: 9, usd_at_time: 3, assets: [], transactions: [] } as unknown as FlowEdge] });
    const twice = preferAtTimeValues(once);
    expect(twice.edges?.[0].usd_spot).toBe(3);
    expect(twice.edges?.[0].usd_now).toBe(9);
  });
});
