import { useState } from "react";
import { toLocalInputValue } from "../lib/format";
import { Segmented } from "./Segmented";

type PresetKey = "1d" | "7d" | "30d" | "90d" | "custom";

const PRESETS: Array<{ value: Exclude<PresetKey, "custom">; label: string; title: string; days: number }> = [
  { value: "1d", label: "24h", title: "Last 24 hours", days: 1 },
  { value: "7d", label: "7d", title: "Last 7 days", days: 7 },
  { value: "30d", label: "30d", title: "Last 30 days", days: 30 },
  { value: "90d", label: "90d", title: "Last 90 days", days: 90 },
];

const TOLERANCE_MS = 15 * 60 * 1000;

interface TimeWindowFieldProps {
  start: string;
  end: string;
  onChange: (next: { start: string; end: string }) => void;
  // Trace windows are entered in UTC; graph windows in local time.
  utc?: boolean;
  now?: () => Date;
}

function toInput(date: Date, utc: boolean) {
  return utc ? date.toISOString().slice(0, 16) : toLocalInputValue(date);
}

function fromInput(value: string, utc: boolean) {
  if (!value) {
    return null;
  }
  const date = new Date(utc ? `${value}:00Z` : value);
  return Number.isNaN(date.getTime()) ? null : date;
}

export function presetForWindow(start: string, end: string, utc: boolean, now = new Date()): PresetKey {
  const startDate = fromInput(start, utc);
  const endDate = fromInput(end, utc);
  if (!startDate || !endDate || Math.abs(now.getTime() - endDate.getTime()) > TOLERANCE_MS) {
    return "custom";
  }
  const span = endDate.getTime() - startDate.getTime();
  const match = PRESETS.find((preset) => Math.abs(span - preset.days * 86_400_000) <= TOLERANCE_MS);
  return match?.value ?? "custom";
}

// TimeWindowField offers the common look-back windows as one click and keeps
// exact start and end inputs for anything else.
export function TimeWindowField({ start, end, onChange, utc = false, now = () => new Date() }: TimeWindowFieldProps) {
  const detected = presetForWindow(start, end, utc, now());
  const [showCustom, setShowCustom] = useState(detected === "custom");
  const mode: PresetKey = showCustom ? "custom" : detected;
  const suffix = utc ? " (UTC)" : "";

  return (
    <div className="field">
      <span>Time window</span>
      <Segmented<PresetKey>
        label="Time window"
        block
        value={mode}
        options={[...PRESETS, { value: "custom", label: "Custom", title: "Choose exact start and end times" }]}
        onChange={(value) => {
          if (value === "custom") {
            setShowCustom(true);
            return;
          }
          setShowCustom(false);
          const preset = PRESETS.find((item) => item.value === value)!;
          const endDate = now();
          const startDate = new Date(endDate.getTime() - preset.days * 86_400_000);
          onChange({ start: toInput(startDate, utc), end: toInput(endDate, utc) });
        }}
      />
      {mode === "custom" ? (
        <div className="field-row">
          <label className="field">
            <span>Start{suffix}</span>
            <input type="datetime-local" value={start} onChange={(event) => onChange({ start: event.target.value, end })} />
          </label>
          <label className="field">
            <span>End{suffix}</span>
            <input type="datetime-local" value={end} onChange={(event) => onChange({ start, end: event.target.value })} />
          </label>
        </div>
      ) : null}
    </div>
  );
}
