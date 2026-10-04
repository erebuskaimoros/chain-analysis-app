import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GraphTimeScrubber } from "../GraphTimeScrubber";
import { formatShortDateTime } from "../../../lib/format";
import type { GraphFilterState } from "../../../lib/graph";

const GRAPH_MIN_TIME = "2026-01-01T00:00:00Z";
const GRAPH_MAX_TIME = "2026-01-11T00:00:00Z";

function makeFilterState(overrides: Partial<GraphFilterState> = {}): GraphFilterState {
  return {
    initialized: true,
    isOpen: false,
    txnTypes: {
      bond_unbond: true,
      rebond: true,
      transfer: true,
      swap: true,
    },
    availableChains: [],
    selectedChains: [],
    graphMinTime: GRAPH_MIN_TIME,
    graphMaxTime: GRAPH_MAX_TIME,
    graphMinTxnUSD: null,
    graphMaxTxnUSD: null,
    startTime: GRAPH_MIN_TIME,
    endTime: GRAPH_MAX_TIME,
    minTxnUSD: null,
    maxTxnUSD: null,
    ...overrides,
  };
}

describe("GraphTimeScrubber", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("syncs to external filter changes after a drag ends outside the inputs", () => {
    const filterState = makeFilterState();
    const { rerender } = render(
      <GraphTimeScrubber filterState={filterState} onStartTimeChange={vi.fn()} onEndTimeChange={vi.fn()} />
    );

    const startInput = screen.getByLabelText("Timeline start");
    fireEvent.mouseDown(startInput);
    fireEvent.mouseUp(window);

    const midMs = (Date.parse(GRAPH_MIN_TIME) + Date.parse(GRAPH_MAX_TIME)) / 2;
    const nextFilterState = makeFilterState({ endTime: new Date(midMs).toISOString() });
    rerender(
      <GraphTimeScrubber filterState={nextFilterState} onStartTimeChange={vi.fn()} onEndTimeChange={vi.fn()} />
    );

    const maxLabel = formatShortDateTime(GRAPH_MAX_TIME);
    const expectedEndLabel = formatShortDateTime(new Date(midMs).toISOString());

    expect(screen.getByText(expectedEndLabel)).toBeTruthy();
    expect(expectedEndLabel).not.toBe(maxLabel);
    expect(screen.queryByText(maxLabel)).toBeNull();
  });

  it("debounces slider changes into onEndTimeChange", () => {
    vi.useFakeTimers();
    const onEndTimeChange = vi.fn();
    const filterState = makeFilterState();
    render(
      <GraphTimeScrubber filterState={filterState} onStartTimeChange={vi.fn()} onEndTimeChange={onEndTimeChange} />
    );

    const endInput = screen.getByLabelText("Timeline end");
    fireEvent.change(endInput, { target: { value: "500" } });

    expect(onEndTimeChange).not.toHaveBeenCalled();
    vi.advanceTimersByTime(250);

    expect(onEndTimeChange).toHaveBeenCalledTimes(1);
    const calledWith = onEndTimeChange.mock.calls[0][0] as string;
    const calledMs = Date.parse(calledWith);
    const minMs = Date.parse(GRAPH_MIN_TIME);
    const maxMs = Date.parse(GRAPH_MAX_TIME);
    const midMs = (minMs + maxMs) / 2;

    expect(calledMs).toBeGreaterThan(minMs);
    expect(calledMs).toBeLessThan(maxMs);
    expect(Math.abs(calledMs - midMs)).toBeLessThan((maxMs - minMs) / 1000 + 1);
  });
});
