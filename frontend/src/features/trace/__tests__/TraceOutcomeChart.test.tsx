import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { TraceEndpoint, TraceResponse } from "../../../lib/types";
import { groupWarnings } from "../../shared/workspace/MapOverlays";
import { outcomeGroup, TraceOutcomeChart } from "../TraceOutcomeChart";

function endpoint(overrides: Partial<TraceEndpoint>): TraceEndpoint {
  return {
    node_id: "n",
    label: "Endpoint",
    kind: "external_address",
    reason: "held",
    depth: 1,
    assets: [],
    traced_usd_at_time: 0,
    confidence: 1,
    first_at: "",
    last_at: "",
    ...overrides,
  };
}

describe("outcomeGroup", () => {
  it("files endpoints by what they are", () => {
    expect(outcomeGroup(endpoint({ category: "sanctioned" }), false)).toBe("flagged");
    expect(outcomeGroup(endpoint({ category: "mixer" }), false)).toBe("flagged");
    expect(outcomeGroup(endpoint({ category: "exchange" }), false)).toBe("exchange");
    expect(outcomeGroup(endpoint({ reason: "pool" }), false)).toBe("protocol");
    expect(outcomeGroup(endpoint({ reason: "held" }), false)).toBe("held");
    expect(outcomeGroup(endpoint({ reason: "contract", category: "exchange" }), false)).toBe("exchange");
    expect(outcomeGroup(endpoint({ reason: "max_depth" }), true)).toBe("limits");
  });
});

describe("TraceOutcomeChart", () => {
  afterEach(() => cleanup());

  it("totals where the value stopped and shows each hop", () => {
    const result = {
      sinks: [
        endpoint({ node_id: "a", category: "exchange", traced_usd_at_time: 600, depth: 1 }),
        endpoint({ node_id: "b", category: "sanctioned", traced_usd_at_time: 300, depth: 2 }),
      ],
      frontier: [endpoint({ node_id: "c", reason: "max_depth", traced_usd_at_time: 100, depth: 2 })],
      edges: [
        { depth: 1, traced_usd_at_time: 1000 },
        { depth: 2, traced_usd_at_time: 400 },
      ],
    } as unknown as TraceResponse;

    render(<TraceOutcomeChart result={result} />);

    const legend = screen.getByRole("heading", { name: "Where it ended up" }).closest("section") as HTMLElement;
    expect(within(legend).getByText("Exchanges")).toBeTruthy();
    expect(within(legend).getByText("$600")).toBeTruthy();
    expect(within(legend).getByText("60%")).toBeTruthy();
    expect(within(legend).getByText("Flagged addresses")).toBeTruthy();
    expect(within(legend).getByText("Stopped by limits")).toBeTruthy();
    expect(screen.getByRole("img").getAttribute("aria-label")).toContain("Exchanges $600");

    expect(screen.getByText("Hop 1")).toBeTruthy();
    expect(screen.getByText("Hop 2")).toBeTruthy();
  });
});

describe("groupWarnings", () => {
  it("folds repeated per-address warnings into one line", () => {
    const groups = groupWarnings([
      "LTC tracker flow truncated for ltc1aaa",
      "LTC tracker flow truncated for ltc1bbb",
      "BASE tracker skipped degraded provider(s): blockscout",
    ]);
    expect(groups).toEqual([
      {
        text: "LTC tracker flow truncated for 2 addresses",
        items: ["LTC tracker flow truncated for ltc1aaa", "LTC tracker flow truncated for ltc1bbb"],
      },
      { text: "BASE tracker skipped degraded provider(s): blockscout", items: [] },
    ]);
  });
});
