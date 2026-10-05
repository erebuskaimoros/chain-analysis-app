import { useEffect, useId, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { CloseIcon } from "../app/icons";

interface DialogProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  className?: string;
  // Hide the visible title (still announced) for dialogs that lead with an input.
  hideTitle?: boolean;
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function Dialog({ title, onClose, children, footer, className = "", hideTitle = false }: DialogProps) {
  const titleID = useId();
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = dialogRef.current;
    if (dialog && !dialog.contains(document.activeElement)) {
      const autofocus = dialog.querySelector<HTMLElement>("[autofocus]");
      (autofocus ?? dialog.querySelector<HTMLElement>(FOCUSABLE) ?? dialog).focus();
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.stopPropagation();
        onCloseRef.current();
        return;
      }
      if (event.key !== "Tab" || !dialogRef.current) {
        return;
      }
      // Keep keyboard focus inside the dialog.
      const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (!focusable.length) {
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKeyDown, true);
    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      previouslyFocused?.focus?.();
    };
  }, []);

  return createPortal(
    <>
      <div className="scrim" onMouseDown={() => onCloseRef.current()} />
      <div
        ref={dialogRef}
        className={`dialog ${className}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        tabIndex={-1}
      >
        <div className={hideTitle ? "visually-hidden" : "dialog-head"}>
          <h2 id={titleID}>{title}</h2>
          {hideTitle ? null : (
            <button type="button" className="btn btn-ghost btn-icon btn-sm" aria-label="Close" onClick={onClose}>
              <CloseIcon />
            </button>
          )}
        </div>
        {hideTitle ? children : <div className="dialog-body">{children}</div>}
        {footer ? <div className="dialog-foot">{footer}</div> : null}
      </div>
    </>,
    document.body
  );
}

interface ConfirmDialogProps {
  title: string;
  message: ReactNode;
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  danger?: boolean;
}

export function ConfirmDialog({ title, message, confirmLabel, onConfirm, onCancel, danger = true }: ConfirmDialogProps) {
  return (
    <Dialog
      title={title}
      onClose={onCancel}
      footer={
        <>
          <button type="button" className="btn" onClick={onCancel} autoFocus>
            Cancel
          </button>
          <button type="button" className={`btn btn-primary${danger ? " btn-danger" : ""}`} onClick={onConfirm}>
            {confirmLabel}
          </button>
        </>
      }
    >
      <div>{message}</div>
    </Dialog>
  );
}
