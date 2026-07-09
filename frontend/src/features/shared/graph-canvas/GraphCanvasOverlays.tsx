import type { ReactNode, RefObject } from "react";
import type { VisibleGraphNode } from "../../../lib/graph";
import type {
  ContextMenuState,
  GraphCanvasFilters,
  GraphCanvasNodeMenuActions,
  GraphCanvasPaneMenuActions,
  GraphHoverCardState,
  GraphWheelMode,
} from "./types";

interface GraphSearchControls {
  query: string;
  setQuery: (value: string) => void;
  matches: VisibleGraphNode[];
  activeIndex: number;
  next: () => void;
  prev: () => void;
  clear: () => void;
}

interface GraphCanvasOverlaysProps {
  filters?: GraphCanvasFilters;
  filterPopoverRef: RefObject<HTMLDivElement>;
  menuRef: RefObject<HTMLDivElement>;
  menuState: ContextMenuState;
  nodeMenuActions?: GraphCanvasNodeMenuActions;
  paneMenuActions?: GraphCanvasPaneMenuActions;
  doubleActivateLabel: string;
  showSaveState: boolean;
  search: GraphSearchControls;
  searchInputRef: RefObject<HTMLInputElement>;
  wheelMode: GraphWheelMode;
  onCycleWheelMode: () => void;
  hoverCard: GraphHoverCardState | null;
  minimapCanvasRef: RefObject<HTMLCanvasElement>;
  onToolbarAction: (action: "zoom-in" | "zoom-out" | "fit" | "fullscreen" | "filters" | "save") => void;
  onContextMenuAction: (action: string) => void;
}

const WHEEL_MODE_LABEL: Record<GraphWheelMode, string> = {
  auto: "Auto",
  zoom: "Zoom",
  pan: "Pan",
};

const WHEEL_MODE_TITLE: Record<GraphWheelMode, string> = {
  auto: "Scroll wheel: Auto (detect mouse vs trackpad) — click to switch",
  zoom: "Scroll wheel: Always zoom — click to switch",
  pan: "Scroll wheel: Always pan — click to switch",
};

export function GraphCanvasOverlays({
  filters,
  filterPopoverRef,
  menuRef,
  menuState,
  nodeMenuActions,
  paneMenuActions,
  doubleActivateLabel,
  showSaveState,
  search,
  searchInputRef,
  wheelMode,
  onCycleWheelMode,
  hoverCard,
  minimapCanvasRef,
  onToolbarAction,
  onContextMenuAction,
}: GraphCanvasOverlaysProps) {
  return (
    <>
      <div className="graph-search">
        <input
          ref={searchInputRef}
          type="text"
          placeholder="Search nodes (/)"
          value={search.query}
          onChange={(event) => search.setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              if (event.shiftKey) {
                search.prev();
              } else {
                search.next();
              }
            } else if (event.key === "Escape") {
              event.preventDefault();
              event.stopPropagation();
              search.clear();
              event.currentTarget.blur();
            }
          }}
        />
        {search.query ? (
          <span className="graph-search-count">
            {search.matches.length ? `${search.activeIndex + 1}/${search.matches.length}` : "0"}
          </span>
        ) : null}
        {search.query ? (
          <button type="button" className="graph-search-clear" title="Clear search" onClick={search.clear}>
            ×
          </button>
        ) : null}
      </div>

      <div className="graph-toolbar">
        {showSaveState ? (
          <ToolbarButton title="Save graph state" onClick={() => onToolbarAction("save")}>
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M3 2h8l2 2v10H3z" />
              <path d="M5 2v4h6V2" />
              <path d="M5 11h6" />
            </svg>
          </ToolbarButton>
        ) : null}
        {filters ? (
          <ToolbarButton
            active={filters.isActive || filters.isOpen}
            title="Filters"
            onClick={() => onToolbarAction("filters")}
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M2 3h12M4 8h8M6 13h4" />
            </svg>
          </ToolbarButton>
        ) : null}
        <ToolbarButton title="Zoom in (+)" onClick={() => onToolbarAction("zoom-in")}>
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
            <line x1="8" y1="3" x2="8" y2="13" />
            <line x1="3" y1="8" x2="13" y2="8" />
          </svg>
        </ToolbarButton>
        <ToolbarButton title="Zoom out (-)" onClick={() => onToolbarAction("zoom-out")}>
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
            <line x1="3" y1="8" x2="13" y2="8" />
          </svg>
        </ToolbarButton>
        <ToolbarButton title="Fit to view (0)" onClick={() => onToolbarAction("fit")}>
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4" />
          </svg>
        </ToolbarButton>
        <ToolbarButton title="Fullscreen (F)" onClick={() => onToolbarAction("fullscreen")}>
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M2 5V2h3M11 2h3v3M14 11v3h-3M5 14H2v-3" />
          </svg>
        </ToolbarButton>
        <button
          type="button"
          className={`wheel-mode${wheelMode !== "auto" ? " is-active" : ""}`}
          title={WHEEL_MODE_TITLE[wheelMode]}
          onClick={onCycleWheelMode}
        >
          {WHEEL_MODE_LABEL[wheelMode]}
        </button>
      </div>

      {filters?.isOpen ? (
        <div className="graph-filter-popover" ref={filterPopoverRef}>
          {filters.content}
        </div>
      ) : null}

      <canvas className="graph-minimap" ref={minimapCanvasRef} width={168} height={112} />

      {hoverCard ? <GraphHoverCard hoverCard={hoverCard} /> : null}

      <div className="graph-help">
        Wheel or pinch to zoom · Trackpad scroll, middle-drag, or space+drag to pan · Left-drag to box-select ·
        Right-click for actions · Double-click to {doubleActivateLabel.toLowerCase()}
      </div>

      {menuState ? (
        <div
          className="graph-context-menu"
          ref={menuRef}
          style={{ left: `${menuState.x}px`, top: `${menuState.y}px` }}
        >
          {menuState.mode === "nodes" ? (
            <>
              <ContextAction
                label={`Expand Nodes (${menuState.nodes.length})`}
                onClick={() => onContextMenuAction("expand-nodes")}
              />
              <ContextAction label="Cluster Nodes" onClick={() => onContextMenuAction("cluster-nodes")} />
            </>
          ) : null}
          {menuState.mode === "node" ? (
            <>
              {nodeMenuActions?.onOpenExplorer ? (
                <ContextAction label="Open explorer" onClick={() => onContextMenuAction("explorer")} />
              ) : null}
              {nodeMenuActions?.onCopyAddress ? (
                <ContextAction label="Copy address" onClick={() => onContextMenuAction("copy-address")} />
              ) : null}
              {nodeMenuActions?.onRefreshLiveValue ? (
                <ContextAction label="Refresh live value" onClick={() => onContextMenuAction("refresh-live-value")} />
              ) : null}
              <ContextAction label={doubleActivateLabel} onClick={() => onContextMenuAction("expand-hop")} />
              {nodeMenuActions?.onLabelNode ? (
                <ContextAction label="Label node" onClick={() => onContextMenuAction("label-node")} />
              ) : null}
              {nodeMenuActions?.onMarkAsgard ? (
                <ContextAction label="Mark Asgard" onClick={() => onContextMenuAction("mark-asgard")} />
              ) : null}
              {nodeMenuActions?.onRemoveNode ? (
                <ContextAction label="Remove from graph" onClick={() => onContextMenuAction("remove-node")} />
              ) : null}
            </>
          ) : null}
          {menuState.mode === "pane" && paneMenuActions?.onCheckUnavailable ? (
            <ContextAction label="Check unavailable live values" onClick={() => onContextMenuAction("check-unavailable")} />
          ) : null}
        </div>
      ) : null}
    </>
  );
}

function GraphHoverCard({ hoverCard }: { hoverCard: GraphHoverCardState }) {
  const { node, x, y } = hoverCard;
  const address = node.metrics && typeof node.metrics === "object" ? String(node.metrics.address ?? "") : "";
  const flowUSD = node.metrics && typeof node.metrics === "object" ? Number(node.metrics.usd_spot ?? 0) : 0;
  return (
    <div className="graph-hover-card" style={{ left: `${x + 14}px`, top: `${y + 14}px` }}>
      <strong>{node.displayLabel || node.label || node.id}</strong>
      <span className="graph-hover-kind">
        {node.kind}
        {node.chain ? ` · ${node.chain}` : ""}
      </span>
      {address ? <code>{address}</code> : null}
      {node.live_holdings_label ? <span>Live: {node.live_holdings_label}</span> : null}
      {flowUSD > 0 ? <span>Flow: {formatHoverUSD(flowUSD)}</span> : null}
    </div>
  );
}

function formatHoverUSD(value: number) {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(value);
}

function ToolbarButton({
  active = false,
  title,
  onClick,
  children,
}: {
  active?: boolean;
  title: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button type="button" className={active ? "is-active" : ""} title={title} onClick={onClick}>
      {children}
    </button>
  );
}

function ContextAction({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick}>
      {label}
    </button>
  );
}
