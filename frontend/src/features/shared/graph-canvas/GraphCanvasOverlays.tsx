import type { ReactNode, RefObject } from "react";
import type { ContextMenuState, GraphCanvasFilters, GraphCanvasNodeMenuActions, GraphCanvasPaneMenuActions } from "./types";

interface GraphCanvasOverlaysProps {
  filters?: GraphCanvasFilters;
  filterPopoverRef: RefObject<HTMLDivElement>;
  menuRef: RefObject<HTMLDivElement>;
  menuState: ContextMenuState;
  nodeMenuActions?: GraphCanvasNodeMenuActions;
  paneMenuActions?: GraphCanvasPaneMenuActions;
  doubleActivateLabel: string;
  onToolbarAction: (action: "zoom-in" | "zoom-out" | "fit" | "fullscreen" | "filters") => void;
  onContextMenuAction: (action: string) => void;
}

export function GraphCanvasOverlays({
  filters,
  filterPopoverRef,
  menuRef,
  menuState,
  nodeMenuActions,
  paneMenuActions,
  doubleActivateLabel,
  onToolbarAction,
  onContextMenuAction,
}: GraphCanvasOverlaysProps) {
  return (
    <>
      <div className="graph-toolbar">
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
      </div>

      {filters?.isOpen ? (
        <div className="graph-filter-popover" ref={filterPopoverRef}>
          {filters.content}
        </div>
      ) : null}

      <div className="graph-help">
        Wheel to zoom · Left-drag to box-select · Middle-drag to pan · Right-click for actions · Double-click to{" "}
        {doubleActivateLabel.toLowerCase()}
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
