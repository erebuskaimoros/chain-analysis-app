import { useEffect, useRef, useState, type ReactNode } from "react";
import { ChevronUpIcon, CloseIcon } from "../../../app/icons";
import { usePreference } from "../../../app/preferences";

interface WorkspacePanel {
  title: ReactNode;
  content: ReactNode;
  open: boolean;
  onClose: () => void;
  // Optional row under the panel title (tabs).
  subheader?: ReactNode;
  closeLabel?: string;
}

interface WorkspaceTray {
  title: string;
  count?: number;
  extra?: ReactNode;
  content: ReactNode;
  storageKey: string;
}

interface GraphWorkspaceProps {
  left: WorkspacePanel;
  right: WorkspacePanel;
  canvas: ReactNode;
  tray?: WorkspaceTray;
}

const TRAY_MIN = 120;
const TRAY_DEFAULT = 260;

// GraphWorkspace gives the map the page: the query panel on the left, the
// inspector on the right when something is selected, and a tray of
// supporting actions along the bottom.
export function GraphWorkspace({ left, right, canvas, tray }: GraphWorkspaceProps) {
  return (
    <div className={`ws${left.open ? " has-left" : ""}${right.open ? " has-right" : ""}`}>
      <aside className="ws-left" aria-label={typeof left.title === "string" ? left.title : undefined} aria-hidden={!left.open}>
        {left.open ? <PanelChrome panel={left} /> : null}
      </aside>
      <div className="ws-canvas">{canvas}</div>
      <aside className="ws-right" aria-label="Inspector" aria-hidden={!right.open}>
        {right.open ? <PanelChrome panel={right} /> : null}
      </aside>
      {tray ? <Tray tray={tray} /> : null}
    </div>
  );
}

function PanelChrome({ panel }: { panel: WorkspacePanel }) {
  return (
    <>
      <div className="ws-panel-head">
        <h2>{panel.title}</h2>
        <button
          type="button"
          className="btn btn-ghost btn-icon btn-sm"
          aria-label={panel.closeLabel ?? "Close panel"}
          title={panel.closeLabel ?? "Close panel"}
          onClick={panel.onClose}
        >
          <CloseIcon />
        </button>
      </div>
      {panel.subheader}
      <div className="ws-panel-body">{panel.content}</div>
    </>
  );
}

function Tray({ tray }: { tray: WorkspaceTray }) {
  const [open, setOpen] = usePreference<boolean>(`${tray.storageKey}.open`, false);
  const [height, setHeight] = usePreference<number>(`${tray.storageKey}.height`, TRAY_DEFAULT);
  const [dragging, setDragging] = useState(false);
  const dragRef = useRef<{ startY: number; startHeight: number } | null>(null);

  useEffect(() => {
    if (!dragging) {
      return;
    }
    function onMove(event: PointerEvent) {
      const drag = dragRef.current;
      if (!drag) {
        return;
      }
      const max = Math.max(TRAY_MIN, window.innerHeight * 0.65);
      const next = Math.min(max, Math.max(TRAY_MIN, drag.startHeight + (drag.startY - event.clientY)));
      setHeight(Math.round(next));
    }
    function onUp() {
      setDragging(false);
      dragRef.current = null;
    }
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
  }, [dragging, setHeight]);

  return (
    <section className={`ws-tray${open ? " is-open" : ""}`} aria-label={tray.title}>
      {open ? (
        <div
          className={`ws-tray-resize${dragging ? " is-dragging" : ""}`}
          role="separator"
          aria-orientation="horizontal"
          aria-label="Resize the tray"
          onPointerDown={(event) => {
            event.preventDefault();
            dragRef.current = { startY: event.clientY, startHeight: height };
            setDragging(true);
          }}
        />
      ) : null}
      <div className="ws-tray-bar">
        <button type="button" className="ws-tray-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>
          <ChevronUpIcon />
          {tray.title}
          {tray.count !== undefined ? <span className="count">{tray.count}</span> : null}
        </button>
        <div className="ws-tray-extra">{tray.extra}</div>
      </div>
      {open ? (
        <div className="ws-tray-body" style={{ height }}>
          {tray.content}
        </div>
      ) : null}
    </section>
  );
}
