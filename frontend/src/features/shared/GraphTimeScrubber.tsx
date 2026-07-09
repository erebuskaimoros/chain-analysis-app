import { useEffect, useMemo, useRef, useState } from "react";
import { formatShortDateTime } from "../../lib/format";
import type { GraphFilterState } from "../../lib/graph";

const SCRUBBER_STEPS = 1000;
const APPLY_DEBOUNCE_MS = 200;

interface GraphTimeScrubberProps {
  filterState: GraphFilterState;
  onStartTimeChange: (value: string) => void;
  onEndTimeChange: (value: string) => void;
}

// Dual-handle timeline under the graph. Dragging the handles narrows the
// existing time filter, so the canvas animates through time without opening
// the filter popover.
export function GraphTimeScrubber({ filterState, onStartTimeChange, onEndTimeChange }: GraphTimeScrubberProps) {
  const minMs = useMemo(() => parseISOMs(filterState.graphMinTime), [filterState.graphMinTime]);
  const maxMs = useMemo(() => parseISOMs(filterState.graphMaxTime), [filterState.graphMaxTime]);
  const startMs = parseISOMs(filterState.startTime) ?? minMs;
  const endMs = parseISOMs(filterState.endTime) ?? maxMs;

  const [dragging, setDragging] = useState(false);
  const [localStart, setLocalStart] = useState(startMs);
  const [localEnd, setLocalEnd] = useState(endMs);
  const applyTimerRef = useRef<number | null>(null);

  useEffect(() => {
    if (!dragging) {
      setLocalStart(startMs);
      setLocalEnd(endMs);
    }
  }, [dragging, endMs, startMs]);

  useEffect(
    () => () => {
      if (applyTimerRef.current !== null) {
        window.clearTimeout(applyTimerRef.current);
      }
    },
    []
  );

  if (minMs === null || maxMs === null || maxMs <= minMs) {
    return null;
  }

  const span = maxMs - minMs;
  const toStep = (ms: number | null) =>
    ms === null ? 0 : Math.round(((clampMs(ms, minMs, maxMs) - minMs) / span) * SCRUBBER_STEPS);
  const fromStep = (step: number) => minMs + (step / SCRUBBER_STEPS) * span;

  const startStep = toStep(localStart);
  const endStep = toStep(localEnd);

  function scheduleApply(nextStart: number, nextEnd: number) {
    if (applyTimerRef.current !== null) {
      window.clearTimeout(applyTimerRef.current);
    }
    applyTimerRef.current = window.setTimeout(() => {
      applyTimerRef.current = null;
      onStartTimeChange(new Date(nextStart).toISOString());
      onEndTimeChange(new Date(nextEnd).toISOString());
    }, APPLY_DEBOUNCE_MS);
  }

  function onStartInput(step: number) {
    const bounded = Math.min(step, endStep);
    const next = fromStep(bounded);
    setLocalStart(next);
    scheduleApply(next, localEnd ?? maxMs!);
  }

  function onEndInput(step: number) {
    const bounded = Math.max(step, startStep);
    const next = fromStep(bounded);
    setLocalEnd(next);
    scheduleApply(localStart ?? minMs!, next);
  }

  const windowLeftPct = (startStep / SCRUBBER_STEPS) * 100;
  const windowWidthPct = Math.max(0, ((endStep - startStep) / SCRUBBER_STEPS) * 100);

  return (
    <div className="graph-timeline">
      <div className="graph-timeline-track">
        <div className="graph-timeline-rail" />
        <div
          className="graph-timeline-window"
          style={{ left: `${windowLeftPct}%`, width: `${windowWidthPct}%` }}
        />
        <input
          type="range"
          min={0}
          max={SCRUBBER_STEPS}
          value={startStep}
          aria-label="Timeline start"
          onMouseDown={() => setDragging(true)}
          onMouseUp={() => setDragging(false)}
          onTouchStart={() => setDragging(true)}
          onTouchEnd={() => setDragging(false)}
          onChange={(event) => onStartInput(Number(event.target.value))}
        />
        <input
          type="range"
          min={0}
          max={SCRUBBER_STEPS}
          value={endStep}
          aria-label="Timeline end"
          onMouseDown={() => setDragging(true)}
          onMouseUp={() => setDragging(false)}
          onTouchStart={() => setDragging(true)}
          onTouchEnd={() => setDragging(false)}
          onChange={(event) => onEndInput(Number(event.target.value))}
        />
      </div>
      <div className="graph-timeline-labels">
        <span>{formatShortDateTime(new Date(localStart ?? minMs).toISOString())}</span>
        <span>{formatShortDateTime(new Date(localEnd ?? maxMs).toISOString())}</span>
      </div>
    </div>
  );
}

function parseISOMs(value: string) {
  if (!value) {
    return null;
  }
  const ms = Date.parse(value);
  return Number.isFinite(ms) ? ms : null;
}

function clampMs(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}
