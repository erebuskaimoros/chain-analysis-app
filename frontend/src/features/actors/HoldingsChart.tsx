import { useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";
import { formatShortDateTime } from "../../lib/format";

export interface HoldingsPoint {
  taken_at: string;
  total_usd: number;
}

interface HoldingsChartProps {
  points: HoldingsPoint[];
  title?: string;
}

const WIDTH = 640;
const PLOT_HEIGHT = 180;
const AXIS_BAND = 28;
const LEFT = 64;
const RIGHT = 16;
const TOP = 12;

function formatAxisUSD(value: number) {
  return new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", notation: "compact", maximumFractionDigits: 1 }).format(value);
}

function formatUSD(value: number) {
  return new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 0 }).format(value);
}

function niceTicks(max: number, count = 4) {
  if (max <= 0) {
    return [0];
  }
  const raw = max / count;
  const magnitude = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * magnitude).find((candidate) => candidate >= raw) ?? raw;
  const ticks: number[] = [];
  for (let value = 0; value <= max + step * 0.001; value += step) {
    ticks.push(value);
  }
  return ticks;
}

// HoldingsChart draws one series (an actor's total holdings at each refresh)
// as a line with point markers. A crosshair snaps to the nearest refresh on
// hover or keyboard focus; the table view carries every value.
export function HoldingsChart({ points, title = "Holdings value at each refresh" }: HoldingsChartProps) {
  const [active, setActive] = useState<number | null>(null);
  const [showTable, setShowTable] = useState(false);
  const svgRef = useRef<SVGSVGElement | null>(null);

  const geometry = useMemo(() => {
    const times = points.map((point) => new Date(point.taken_at).getTime());
    const minT = Math.min(...times);
    const maxT = Math.max(...times);
    const ticks = niceTicks(Math.max(...points.map((point) => point.total_usd), 0));
    const maxY = ticks[ticks.length - 1] || 1;
    const x = (t: number) => (maxT === minT ? LEFT + (WIDTH - LEFT - RIGHT) / 2 : LEFT + ((t - minT) / (maxT - minT)) * (WIDTH - LEFT - RIGHT));
    const y = (v: number) => TOP + PLOT_HEIGHT - (v / maxY) * PLOT_HEIGHT;
    return {
      ticks,
      x,
      y,
      coords: points.map((point, index) => ({ x: x(times[index]), y: y(point.total_usd) })),
    };
  }, [points]);

  if (!points.length) {
    return <div className="empty-state">No refreshes yet. Refresh to record the first snapshot.</div>;
  }

  const path = geometry.coords.map((c, index) => `${index === 0 ? "M" : "L"}${c.x.toFixed(1)},${c.y.toFixed(1)}`).join(" ");

  function nearestIndex(clientX: number) {
    const svg = svgRef.current;
    if (!svg) {
      return null;
    }
    const rect = svg.getBoundingClientRect();
    const svgX = ((clientX - rect.left) / rect.width) * WIDTH;
    let best = 0;
    geometry.coords.forEach((c, index) => {
      if (Math.abs(c.x - svgX) < Math.abs(geometry.coords[best].x - svgX)) {
        best = index;
      }
    });
    return best;
  }

  function onPointerMove(event: PointerEvent<SVGSVGElement>) {
    setActive(nearestIndex(event.clientX));
  }

  function onKeyDown(event: KeyboardEvent<SVGSVGElement>) {
    if (event.key === "ArrowRight") {
      setActive((current) => Math.min(points.length - 1, (current ?? -1) + 1));
      event.preventDefault();
    } else if (event.key === "ArrowLeft") {
      setActive((current) => Math.max(0, (current ?? points.length) - 1));
      event.preventDefault();
    }
  }

  const activePoint = active != null ? points[active] : null;
  const activeCoord = active != null ? geometry.coords[active] : null;

  return (
    <figure className="holdings-chart">
      <figcaption className="holdings-chart-head">
        <span className="holdings-chart-title">{title}</span>
        <button type="button" className="btn btn-sm" onClick={() => setShowTable((value) => !value)}>
          {showTable ? "Show chart" : "Show table"}
        </button>
      </figcaption>
      {showTable ? (
        <div className="table-wrap compact">
          <table className="data-table compact">
            <thead>
              <tr>
                <th>Refreshed</th>
                <th className="numeric">Holdings (USD)</th>
              </tr>
            </thead>
            <tbody>
              {points.map((point) => (
                <tr key={point.taken_at}>
                  <td>{formatShortDateTime(point.taken_at)}</td>
                  <td className="numeric">{formatUSD(point.total_usd)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="holdings-chart-plot">
          <svg
            ref={svgRef}
            viewBox={`0 0 ${WIDTH} ${TOP + PLOT_HEIGHT + AXIS_BAND}`}
            role="img"
            aria-label={`${title}: ${points.length} refreshes, latest ${formatUSD(points[points.length - 1].total_usd)}`}
            tabIndex={0}
            onPointerMove={onPointerMove}
            onPointerLeave={() => setActive(null)}
            onFocus={() => setActive(points.length - 1)}
            onBlur={() => setActive(null)}
            onKeyDown={onKeyDown}
          >
            {geometry.ticks.map((tick) => (
              <g key={tick}>
                <line className="holdings-chart-grid" x1={LEFT} x2={WIDTH - RIGHT} y1={geometry.y(tick)} y2={geometry.y(tick)} />
                <text className="holdings-chart-axis" x={LEFT - 8} y={geometry.y(tick)} dy="0.32em" textAnchor="end">
                  {formatAxisUSD(tick)}
                </text>
              </g>
            ))}
            <text className="holdings-chart-axis" x={geometry.coords[0].x} y={TOP + PLOT_HEIGHT + 20} textAnchor="start">
              {formatShortDateTime(points[0].taken_at)}
            </text>
            {points.length > 1 ? (
              <text className="holdings-chart-axis" x={geometry.coords[points.length - 1].x} y={TOP + PLOT_HEIGHT + 20} textAnchor="end">
                {formatShortDateTime(points[points.length - 1].taken_at)}
              </text>
            ) : null}
            {activeCoord ? (
              <line className="holdings-chart-crosshair" x1={activeCoord.x} x2={activeCoord.x} y1={TOP} y2={TOP + PLOT_HEIGHT} />
            ) : null}
            <path className="holdings-chart-line" d={path} />
            {geometry.coords.map((c, index) => (
              <circle key={points[index].taken_at} className={`holdings-chart-point${index === active ? " active" : ""}`} cx={c.x} cy={c.y} r={index === active ? 5 : 4} />
            ))}
          </svg>
          {activePoint && activeCoord ? (
            <div
              className="holdings-chart-tooltip"
              style={{ left: `${(activeCoord.x / WIDTH) * 100}%`, top: `${(activeCoord.y / (TOP + PLOT_HEIGHT + AXIS_BAND)) * 100}%` }}
              role="status"
            >
              <span>{formatShortDateTime(activePoint.taken_at)}</span>
              <strong>{formatUSD(activePoint.total_usd)}</strong>
            </div>
          ) : null}
        </div>
      )}
    </figure>
  );
}
