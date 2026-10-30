import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { MoreIcon } from "../app/icons";

export type MenuItem =
  | {
      label: string;
      onSelect: () => void;
      icon?: ReactNode;
      danger?: boolean;
      disabled?: boolean;
    }
  | "separator";

interface MenuButtonProps {
  label: string;
  items: MenuItem[];
  // Button content; defaults to a "more" icon.
  children?: ReactNode;
  className?: string;
  align?: "left" | "right";
  placement?: "below" | "above";
}

export function MenuButton({
  label,
  items,
  children,
  className = "btn btn-ghost btn-icon btn-sm",
  align = "right",
  placement = "below",
}: MenuButtonProps) {
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLSpanElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuID = useId();

  useEffect(() => {
    if (!open) {
      return;
    }
    menuRef.current?.querySelector<HTMLButtonElement>("button:not([disabled])")?.focus();

    function onPointerDown(event: MouseEvent) {
      if (anchorRef.current && event.target instanceof Node && !anchorRef.current.contains(event.target)) {
        setOpen(false);
      }
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.stopPropagation();
        setOpen(false);
        anchorRef.current?.querySelector<HTMLButtonElement>("button")?.focus();
        return;
      }
      if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
        return;
      }
      const buttons = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>("button:not([disabled])") ?? []);
      if (!buttons.length) {
        return;
      }
      event.preventDefault();
      const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
      const next = event.key === "ArrowDown" ? (index + 1) % buttons.length : (index - 1 + buttons.length) % buttons.length;
      buttons[next].focus();
    }
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown, true);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown, true);
    };
  }, [open]);

  return (
    <span className="menu-anchor" ref={anchorRef}>
      <button
        type="button"
        className={className}
        aria-label={children ? undefined : label}
        title={children ? undefined : label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuID : undefined}
        onClick={(event) => {
          event.stopPropagation();
          setOpen((current) => !current);
        }}
      >
        {children ?? <MoreIcon />}
      </button>
      {open ? (
        <div
          id={menuID}
          ref={menuRef}
          className="menu"
          role="menu"
          aria-label={label}
          style={{
            [align === "right" ? "right" : "left"]: 0,
            ...(placement === "above" ? { bottom: "calc(100% + 4px)" } : { top: "calc(100% + 4px)" }),
          }}
        >
          {items.map((item, index) =>
            item === "separator" ? (
              <div key={`sep-${index}`} className="menu-sep" role="separator" />
            ) : (
              <button
                key={item.label}
                type="button"
                role="menuitem"
                className={item.danger ? "menu-danger" : undefined}
                disabled={item.disabled}
                onClick={(event) => {
                  event.stopPropagation();
                  setOpen(false);
                  item.onSelect();
                }}
              >
                {item.icon}
                {item.label}
              </button>
            )
          )}
        </div>
      ) : null}
    </span>
  );
}
