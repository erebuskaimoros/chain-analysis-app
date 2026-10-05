import { useMemo, useState, type FocusEvent, type PointerEvent, type ReactNode } from "react";
import { AlertIcon } from "../../app/icons";
import { formatCompactUSD, formatUSD } from "../../lib/format";
import type { TraceEndpoint, TraceResponse } from "../../lib/types";

// Endpoint groups, in the order they sit along each bar. The four hued groups
// were validated as an adjacent categorical sequence (light and dark); the two
// neutral groups are de-emphasised on purpose.
export type OutcomeGroup = "flagged" | "exchange" | "protocol" | "held" | "other" | "limits";

const GROUPS: Array<{ key: OutcomeGroup; label: string; className: string; icon?: ReactNode }> = [
  { key: "flagged", label: "Flagged addresses", className: "outcome-flagged", icon: <AlertIcon size={12} /> },
  { key: "exchange", label: "Exchanges", className: "outcome-exchange" },
  { key: "protocol", label: "Pools and protocols", className: "outcome-protocol" },
  { key: "held", label: "Still held", className: "outcome-held" },
  { key: "other", label: "Other endpoints", className: "outcome-other" },
  { key: "limits", label: "Stopped by limits", className: "outcome-limits" },
];

const RISK_CATEGORIES = new Set([
  "sanctioned",
  "scam",
  "hack",
  "ransomware",
  "terrorism",
  "extremism",
  "darknet_market",
  "mixer",
]);
const PROTOCOL_REASONS = new Set(["pool", "bond", "contract", "protocol"]);
const PROTOCOL_CATEGORIES = new Set(["defi", "bridge", "protocol"]);

export function outcomeGroup(endpoint: TraceEndpoint, stoppedByLimits: boolean): OutcomeGroup {
  if (stoppedByLimits) {
    return "limits";
  }
  const category = (endpoint.category ?? "").toLowerCase();
  if (RISK_CATEGORIES.has(category)) {
    return "flagged";
  }
  if (category === "exchange" || category === "wallet_service") {
    return "exchange";
  }
  if (PROTOCOL_REASONS.has(endpoint.reason) || PROTOCOL_CATEGORIES.has(category)) {
    return "protocol";
  }
  if (endpoint.reason === "held" || endpoint.reason === "held_before_window") {
    return "held";
  }
  return "other";
}

interface Segment {
  group: OutcomeGroup;
  usd: number;
  count: number;
}

interface HopRow {
  depth: number;
  arrived: number;
  ended: Segment[];
  continued: number;
}

function summarize(result: TraceResponse) {
  const totals = new Map<OutcomeGroup, Segment>();
  const byHop = new Map<number, Map<OutcomeGroup, Segment>>();
  const add = (endpoint: TraceEndpoint, stopped: boolean) => {
    const group = outcomeGroup(endpoint, stopped);
    const usd = Number(endpoint.traced_usd_at_time || 0);
    const total = totals.get(group) ?? { group, usd: 0, count: 0 };
    total.usd += usd;
    total.count += 1;
    totals.set(group, total);
    const hop = byHop.get(endpoint.depth) ?? new Map<OutcomeGroup, Segment>();
    const segment = hop.get(group) ?? { group, usd: 0, count: 0 };
    segment.usd += usd;
    segment.count += 1;
    hop.set(group, segment);
    byHop.set(endpoint.depth, hop);
  };
  result.sinks.forEach((endpoint) => add(endpoint, false));
  result.frontier.forEach((endpoint) => add(endpoint, true));

  const arrivedByHop = new Map<number, number>();
  result.edges.forEach((edge) => {
    arrivedByHop.set(edge.depth, (arrivedByHop.get(edge.depth) ?? 0) + Number(edge.traced_usd_at_time || 0));
  });
  const depths = Array.from(new Set([...arrivedByHop.keys(), ...byHop.keys()]))
    .filter((depth) => depth > 0)
    .sort((left, right) => left - right);
  const hops: HopRow[] = depths.map((depth) => {
    const ended = GROUPS.map((group) => byHop.get(depth)?.get(group.key)).filter((segment): segment is Segment =>
      Boolean(segment && segment.usd > 0)
    );
    const endedUSD = ended.reduce((sum, segment) => sum + segment.usd, 0);
    const arrived = Math.max(arrivedByHop.get(depth) ?? 0, endedUSD);
    return { depth, arrived, ended, continued: Math.max(0, arrived - endedUSD) };
  });

  const segments = GROUPS.map((group) => totals.get(group.key)).filter((segment): segment is Segment =>
    Boolean(segment && segment.usd > 0)
  );
  return { segments, hops };
}

interface TooltipState {
  x: number;
  y: number;
  title: string;
  value: string;
  detail: string;
}

function groupMeta(key: OutcomeGroup) {
  return GROUPS.find((group) => group.key === key)!;
}

// TraceOutcomeChart answers "where did it end up?" with one part-to-whole
// bar, then shows hop by hop how much stopped and how much kept moving.
export function TraceOutcomeChart({ result }: { result: TraceResponse }) {
  const { segments, hops } = useMemo(() => summarize(result), [result]);
  const [tooltip, setTooltip] = useState<TooltipState | null>(null);
  const endedTotal = segments.reduce((sum, segment) => sum + segment.usd, 0);
  const maxArrived = Math.max(1, ...hops.map((hop) => hop.arrived));

  if (!segments.length && !hops.length) {
    return null;
  }

  function showTip(event: PointerEvent | FocusEvent, title: string, value: number, share: number, detail: string) {
    const host = (event.currentTarget as HTMLElement).closest(".outcome") as HTMLElement | null;
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    const hostRect = host?.getBoundingClientRect();
    const pointer = "clientX" in event ? { x: event.clientX, y: event.clientY } : { x: rect.left + rect.width / 2, y: rect.top };
    setTooltip({
      x: pointer.x - (hostRect?.left ?? 0),
      y: rect.top - (hostRect?.top ?? 0),
      title,
      value: formatUSD(value),
      detail: `${Math.round(share * 100)}% ${detail}`,
    });
  }

  return (
    <div className="outcome" onPointerLeave={() => setTooltip(null)}>
      {segments.length ? (
        <section className="section" aria-labelledby="outcome-ended-title">
          <h3 id="outcome-ended-title" className="section-title">
            Where it ended up
          </h3>
          <div className="outcome-bar" role="img" aria-label={segments.map((s) => `${groupMeta(s.group).label} ${formatUSD(s.usd)}`).join(", ")}>
            {segments.map((segment) => {
              const share = endedTotal ? segment.usd / endedTotal : 0;
              const meta = groupMeta(segment.group);
              return (
                <div
                  key={segment.group}
                  className={`outcome-seg ${meta.className}`}
                  style={{ flexGrow: segment.usd }}
                  tabIndex={0}
                  onPointerMove={(event) => showTip(event, meta.label, segment.usd, share, "of the traced value that stopped")}
                  onFocus={(event) => showTip(event, meta.label, segment.usd, share, "of the traced value that stopped")}
                  onBlur={() => setTooltip(null)}
                />
              );
            })}
          </div>
          <ul className="outcome-legend">
            {segments.map((segment) => {
              const meta = groupMeta(segment.group);
              return (
                <li key={segment.group}>
                  <span className={`outcome-key ${meta.className}`} aria-hidden="true" />
                  {meta.icon ? <span className="outcome-icon">{meta.icon}</span> : null}
                  <span className="outcome-legend-label">{meta.label}</span>
                  <span className="outcome-legend-value num">{formatCompactUSD(segment.usd)}</span>
                  <span className="outcome-legend-share num">
                    {endedTotal ? `${Math.round((segment.usd / endedTotal) * 100)}%` : ""}
                  </span>
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}

      {hops.length ? (
        <section className="section" aria-labelledby="outcome-hops-title">
          <h3 id="outcome-hops-title" className="section-title">
            Hop by hop
          </h3>
          <div className="outcome-hops">
            {hops.map((hop) => (
              <div key={hop.depth} className="outcome-hop">
                <span className="outcome-hop-label">Hop {hop.depth}</span>
                <div className="outcome-hop-track">
                  <div className="outcome-hop-bar" style={{ width: `${(hop.arrived / maxArrived) * 100}%` }}>
                    {hop.ended.map((segment) => {
                      const meta = groupMeta(segment.group);
                      return (
                        <div
                          key={segment.group}
                          className={`outcome-seg ${meta.className}`}
                          style={{ flexGrow: segment.usd }}
                          tabIndex={0}
                          onPointerMove={(event) =>
                            showTip(event, `${meta.label} at hop ${hop.depth}`, segment.usd, hop.arrived ? segment.usd / hop.arrived : 0, `of what reached hop ${hop.depth}`)
                          }
                          onFocus={(event) =>
                            showTip(event, `${meta.label} at hop ${hop.depth}`, segment.usd, hop.arrived ? segment.usd / hop.arrived : 0, `of what reached hop ${hop.depth}`)
                          }
                          onBlur={() => setTooltip(null)}
                        />
                      );
                    })}
                    {hop.continued > 0 ? (
                      <div
                        className="outcome-seg outcome-continued"
                        style={{ flexGrow: hop.continued }}
                        tabIndex={0}
                        onPointerMove={(event) =>
                          showTip(event, `Kept moving after hop ${hop.depth}`, hop.continued, hop.arrived ? hop.continued / hop.arrived : 0, `of what reached hop ${hop.depth}`)
                        }
                        onFocus={(event) =>
                          showTip(event, `Kept moving after hop ${hop.depth}`, hop.continued, hop.arrived ? hop.continued / hop.arrived : 0, `of what reached hop ${hop.depth}`)
                        }
                        onBlur={() => setTooltip(null)}
                      />
                    ) : null}
                  </div>
                </div>
                <span className="outcome-hop-value num">{formatCompactUSD(hop.arrived)}</span>
              </div>
            ))}
          </div>
          <p className="section-note outcome-hop-note">
            Each row is the value that reached that hop. Coloured parts stopped there;
            <span className="outcome-key outcome-continued" aria-hidden="true" /> the pale part moved on to the next hop.
          </p>
        </section>
      ) : null}

      {tooltip ? (
        <div className="outcome-tooltip" style={{ left: tooltip.x, top: tooltip.y }} role="tooltip">
          <strong className="num">{tooltip.value}</strong>
          <span>{tooltip.title}</span>
          <span className="outcome-tooltip-detail">{tooltip.detail}</span>
        </div>
      ) : null}
    </div>
  );
}
