import { useEffect, useState } from "react";
import { Dialog } from "../ui/Dialog";

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
const MOD = isMac ? "⌘" : "Ctrl";

const GROUPS: Array<{ title: string; items: Array<{ label: string; keys: string[] }> }> = [
  {
    title: "Anywhere",
    items: [
      { label: "Search addresses, transactions, actors and cases", keys: [MOD, "K"] },
      { label: "Show these shortcuts", keys: ["?"] },
      { label: "Close a dialog, menu or popover", keys: ["Esc"] },
    ],
  },
  {
    title: "On the graph",
    items: [
      { label: "Find a node", keys: ["/"] },
      { label: "Next match", keys: ["Enter"] },
      { label: "Previous match", keys: ["Shift", "Enter"] },
      { label: "Zoom in or out", keys: ["+", "−"] },
      { label: "Fit the graph to the view", keys: ["0"] },
      { label: "Fullscreen", keys: ["F"] },
      { label: "Pan", keys: ["Space", "drag"] },
      { label: "Select several nodes", keys: ["drag"] },
      { label: "Node actions", keys: ["right-click"] },
      { label: "Expand one edge from a node", keys: ["double-click"] },
    ],
  },
];

const NON_TEXT_INPUTS = new Set(["checkbox", "radio", "button", "submit", "reset", "range", "color", "file"]);

// Shortcuts stand down only while the user is typing text.
function isTextEntry(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  if (target.isContentEditable || target.tagName === "TEXTAREA" || target.tagName === "SELECT") {
    return true;
  }
  return target.tagName === "INPUT" && !NON_TEXT_INPUTS.has((target as HTMLInputElement).type);
}

// ShortcutsDialog opens with "?" and lists every keyboard and mouse shortcut.
export function ShortcutsDialog() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "?" || event.metaKey || event.ctrlKey || event.altKey) {
        return;
      }
      if (isTextEntry(event.target)) {
        return;
      }
      event.preventDefault();
      setOpen((current) => !current);
    }
    function onOpenRequest() {
      setOpen(true);
    }
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("chain-analysis:shortcuts", onOpenRequest);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("chain-analysis:shortcuts", onOpenRequest);
    };
  }, []);

  if (!open) {
    return null;
  }

  return (
    <Dialog title="Keyboard and mouse shortcuts" onClose={() => setOpen(false)}>
      {GROUPS.map((group) => (
        <section key={group.title} className="shortcut-group section">
          <h3 className="section-title">{group.title}</h3>
          <dl className="shortcut-list">
            {group.items.map((item) => (
              <div key={item.label} style={{ display: "contents" }}>
                <dt>{item.label}</dt>
                <dd>
                  {item.keys.map((key) => (
                    <kbd key={key}>{key}</kbd>
                  ))}
                </dd>
              </div>
            ))}
          </dl>
        </section>
      ))}
    </Dialog>
  );
}

export function openShortcuts() {
  window.dispatchEvent(new Event("chain-analysis:shortcuts"));
}
