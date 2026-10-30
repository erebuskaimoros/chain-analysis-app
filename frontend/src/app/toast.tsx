import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";

export interface ToastOptions {
  tone?: "default" | "error";
  actionLabel?: string;
  onAction?: () => void;
  durationMs?: number;
}

interface ToastEntry extends ToastOptions {
  id: number;
  message: string;
}

type ShowToast = (message: string, options?: ToastOptions) => void;

const ToastContext = createContext<ShowToast>(() => undefined);

let toastSeq = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastEntry[]>([]);

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const show = useCallback<ShowToast>((message, options = {}) => {
    toastSeq += 1;
    const entry: ToastEntry = { id: toastSeq, message, ...options };
    // Keep the stack short: the newest three are enough to read.
    setToasts((current) => [...current.slice(-2), entry]);
  }, []);

  return (
    <ToastContext.Provider value={show}>
      {children}
      <div className="toast-stack" role="status" aria-live="polite">
        {toasts.map((toast) => (
          <ToastItem key={toast.id} toast={toast} onDismiss={dismiss} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

function ToastItem({ toast, onDismiss }: { toast: ToastEntry; onDismiss: (id: number) => void }) {
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    timerRef.current = window.setTimeout(() => onDismiss(toast.id), toast.durationMs ?? (toast.onAction ? 7000 : 4500));
    return () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }
    };
  }, [onDismiss, toast.durationMs, toast.id, toast.onAction]);

  return (
    <div className={`toast${toast.tone === "error" ? " toast-error" : ""}`}>
      <span>{toast.message}</span>
      {toast.onAction ? (
        <button
          type="button"
          className="btn"
          onClick={() => {
            toast.onAction?.();
            onDismiss(toast.id);
          }}
        >
          {toast.actionLabel ?? "Undo"}
        </button>
      ) : null}
    </div>
  );
}

export function useToast() {
  return useContext(ToastContext);
}
