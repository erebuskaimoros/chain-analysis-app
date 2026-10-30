import { createContext, useCallback, useContext, useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { listActors, listAnnotations } from "../lib/api";
import { explorerURLForAddress, explorerURLForTx } from "../lib/graph/actions";
import { encodeSeed, identifyInput } from "../lib/identify";
import { middleTruncate, pluralize } from "../lib/format";
import { Dialog } from "../ui/Dialog";
import { useActiveCase } from "./activeCase";
import {
  ActorsIcon,
  CasesIcon,
  CopyIcon,
  ExplorerIcon,
  ExternalIcon,
  GraphIcon,
  PinIcon,
  SearchIcon,
  TagIcon,
  TraceIcon,
} from "./icons";
import { NAV_ITEMS } from "./navigation";
import { useRouter } from "./router";
import { useToast } from "./toast";

export interface SearchAction {
  id: string;
  group: string;
  title: string;
  detail?: string;
  icon: ReactNode;
  run: () => void;
}

const MAX_PER_GROUP = 6;

function matches(haystack: string, needle: string) {
  return haystack.toLowerCase().includes(needle.toLowerCase());
}

// useSearchActions turns whatever was typed or pasted into the things you can
// do with it: explore or trace an address, look up a transaction, open an
// actor or case, or jump to a page.
export function useSearchActions(query: string, enabled: boolean): SearchAction[] {
  const { navigate } = useRouter();
  const { cases, addToCase, setActiveCaseID } = useActiveCase();
  const toast = useToast();
  const actorsQuery = useQuery({ queryKey: ["actors"], queryFn: listActors, enabled });
  const annotationsQuery = useQuery({ queryKey: ["annotations"], queryFn: listAnnotations, enabled });

  return useMemo(() => {
    const identified = identifyInput(query);
    const actions: SearchAction[] = [];
    const text = identified.value;

    if (identified.kind === "tx") {
      const tx = identified.value;
      const short = middleTruncate(tx, 8, 6);
      actions.push(
        {
          id: "tx-lookup",
          group: "Transaction",
          title: "Look up this transaction",
          detail: short,
          icon: <SearchIcon />,
          run: () => navigate("explorer", { tx }),
        },
        {
          id: "tx-trace",
          group: "Transaction",
          title: "Trace funds from this transaction",
          detail: short,
          icon: <TraceIcon />,
          run: () => navigate("trace", { seed: tx }),
        },
        {
          id: "tx-case",
          group: "Transaction",
          title: "Add to the active case",
          detail: short,
          icon: <PinIcon />,
          run: () => addToCase([{ kind: "tx", ref: tx }], short),
        },
        {
          id: "tx-external",
          group: "Transaction",
          title: "Open in a block explorer",
          detail: "thorchain.net",
          icon: <ExternalIcon />,
          run: () => window.open(explorerURLForTx(tx, "THOR"), "_blank", "noopener,noreferrer"),
        }
      );
    }

    if (identified.kind === "address") {
      const { value: address, chain, chainCertain } = identified;
      const short = middleTruncate(address, 8, 6);
      const seed = chainCertain ? encodeSeed(address, chain) : address;
      const chainNote = chainCertain ? `${chain} address` : `Looks like an ${chain}-style address`;
      actions.push(
        {
          id: "addr-explore",
          group: "Address",
          title: "Explore this address",
          detail: `${short}, ${chainNote}`,
          icon: <ExplorerIcon />,
          run: () => navigate("explorer", { address }),
        },
        {
          id: "addr-trace",
          group: "Address",
          title: "Trace funds from this address",
          detail: short,
          icon: <TraceIcon />,
          run: () => navigate("trace", { seed }),
        },
        {
          id: "addr-case",
          group: "Address",
          title: "Add to the active case",
          detail: short,
          icon: <PinIcon />,
          run: () => addToCase([{ kind: "address", ref: seed }], short),
        },
        {
          id: "addr-label",
          group: "Address",
          title: "Label this address",
          detail: short,
          icon: <TagIcon />,
          run: () => navigate("annotations", { address }),
        },
        {
          id: "addr-copy",
          group: "Address",
          title: "Copy address",
          detail: short,
          icon: <CopyIcon />,
          run: () => {
            void navigator.clipboard?.writeText(address).then(() => toast("Copied address"));
          },
        }
      );
      const externalURL = explorerURLForAddress(address, chain);
      if (externalURL) {
        actions.push({
          id: "addr-external",
          group: "Address",
          title: "Open in a block explorer",
          detail: new URL(externalURL).host,
          icon: <ExternalIcon />,
          run: () => window.open(externalURL, "_blank", "noopener,noreferrer"),
        });
      }
    }

    const searchText = identified.kind === "text" ? text : "";
    const showDirectory = identified.kind === "text" || identified.kind === "empty";

    if (showDirectory) {
      const actors = [...(actorsQuery.data ?? [])]
        .filter((actor) => !searchText || matches(actor.name, searchText))
        .sort((left, right) => left.name.localeCompare(right.name))
        .slice(0, searchText ? MAX_PER_GROUP : 4);
      actors.forEach((actor) => {
        actions.push(
          {
            id: `actor-graph-${actor.id}`,
            group: "Actors",
            title: `Graph ${actor.name}`,
            detail: pluralize(actor.addresses.length, "address", "addresses"),
            icon: <GraphIcon />,
            run: () => navigate("graph", { actors: String(actor.id), run: "1" }),
          },
          {
            id: `actor-open-${actor.id}`,
            group: "Actors",
            title: `Open ${actor.name}`,
            detail: actor.notes || "Actor details and monitoring",
            icon: <ActorsIcon />,
            run: () => navigate("actors", { actor: String(actor.id) }),
          }
        );
      });

      if (searchText) {
        const labeled = (annotationsQuery.data ?? [])
          .filter((annotation) => annotation.kind === "label" && annotation.value && matches(annotation.value, searchText))
          .slice(0, MAX_PER_GROUP);
        labeled.forEach((annotation) => {
          actions.push({
            id: `label-${annotation.id}`,
            group: "Labeled addresses",
            title: annotation.value,
            detail: middleTruncate(annotation.address, 10, 8),
            icon: <TagIcon />,
            run: () => navigate("explorer", { address: annotation.address }),
          });
        });
      }

      cases
        .filter((item) => !searchText || matches(item.title, searchText))
        .slice(0, searchText ? MAX_PER_GROUP : 3)
        .forEach((item) => {
          actions.push({
            id: `case-${item.id}`,
            group: "Cases",
            title: item.title,
            detail: pluralize(item.item_count, "item"),
            icon: <CasesIcon />,
            run: () => {
              setActiveCaseID(item.id);
              navigate("cases", { case: String(item.id) });
            },
          });
        });

      NAV_ITEMS.filter((item) => !searchText || matches(item.label, searchText)).forEach((item) => {
        actions.push({
          id: `page-${item.key}`,
          group: "Go to",
          title: item.label,
          icon: item.icon,
          run: () => navigate(item.key),
        });
      });
    }

    return actions;
  }, [actorsQuery.data, addToCase, annotationsQuery.data, cases, navigate, query, setActiveCaseID, toast]);
}

interface OmniboxProps {
  autoFocus?: boolean;
  placeholder?: string;
  onDone?: () => void;
  // Inline omniboxes (Home) only list results once something is typed.
  inline?: boolean;
}

export function Omnibox({ autoFocus = false, placeholder, onDone, inline = false }: OmniboxProps) {
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const listID = useId();
  const listRef = useRef<HTMLDivElement | null>(null);
  const showResults = !inline || query.trim().length > 0;
  const actions = useSearchActions(query, true);
  const visible = showResults ? actions : [];

  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  useEffect(() => {
    listRef.current
      ?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`)
      ?.scrollIntoView?.({ block: "nearest" });
  }, [activeIndex]);

  function runAt(index: number) {
    const action = visible[index];
    if (!action) {
      return;
    }
    action.run();
    setQuery("");
    onDone?.();
  }

  const groups: Array<{ name: string; items: Array<{ action: SearchAction; index: number }> }> = [];
  visible.forEach((action, index) => {
    const group = groups.find((entry) => entry.name === action.group);
    if (group) {
      group.items.push({ action, index });
    } else {
      groups.push({ name: action.group, items: [{ action, index }] });
    }
  });

  return (
    <div className={inline ? "omnibox omnibox-inline" : "omnibox"}>
      <div className="palette-input">
        <SearchIcon size={18} />
        <input
          autoFocus={autoFocus}
          value={query}
          role="combobox"
          aria-expanded={visible.length > 0}
          aria-controls={listID}
          aria-activedescendant={visible[activeIndex] ? `${listID}-${activeIndex}` : undefined}
          aria-label="Search addresses, transactions, actors and cases"
          placeholder={placeholder ?? "Paste an address or transaction hash, or search actors and cases"}
          spellCheck={false}
          autoComplete="off"
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setActiveIndex((current) => (visible.length ? (current + 1) % visible.length : 0));
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              setActiveIndex((current) => (visible.length ? (current - 1 + visible.length) % visible.length : 0));
            } else if (event.key === "Enter") {
              event.preventDefault();
              runAt(activeIndex);
            } else if (event.key === "Escape" && inline && query) {
              event.preventDefault();
              setQuery("");
            }
          }}
        />
      </div>
      {showResults ? (
        <div className="palette-results" id={listID} role="listbox" ref={listRef} aria-label="Search results">
          {groups.length ? (
            groups.map((group) => (
              <div key={group.name} role="group" aria-label={group.name}>
                <div className="palette-group-label">{group.name}</div>
                {group.items.map(({ action, index }) => (
                  <button
                    key={action.id}
                    id={`${listID}-${index}`}
                    type="button"
                    role="option"
                    data-index={index}
                    aria-selected={index === activeIndex}
                    className="palette-item"
                    onMouseMove={() => setActiveIndex(index)}
                    onClick={() => runAt(index)}
                  >
                    <span className="palette-item-icon">{action.icon}</span>
                    <span className="palette-item-main">
                      <span className="palette-item-title">{action.title}</span>
                      {action.detail ? <span className="palette-item-detail">{action.detail}</span> : null}
                    </span>
                  </button>
                ))}
              </div>
            ))
          ) : (
            <div className="palette-empty">
              Nothing matches. Paste a full address or a 64-character transaction hash, or type part of an actor, label or case
              name.
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}

interface SearchPaletteValue {
  openSearch: () => void;
}

const SearchPaletteContext = createContext<SearchPaletteValue>({ openSearch: () => undefined });

export function SearchPaletteProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const openSearch = useCallback(() => setOpen(true), []);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const value = useMemo(() => ({ openSearch }), [openSearch]);

  return (
    <SearchPaletteContext.Provider value={value}>
      {children}
      {open ? (
        <Dialog title="Search" hideTitle className="palette" onClose={() => setOpen(false)}>
          <Omnibox autoFocus onDone={() => setOpen(false)} />
          <div className="palette-foot">
            <span>
              <kbd>↑</kbd>
              <kbd>↓</kbd> to move
            </span>
            <span>
              <kbd>Enter</kbd> to run
            </span>
            <span>
              <kbd>Esc</kbd> to close
            </span>
          </div>
        </Dialog>
      ) : null}
    </SearchPaletteContext.Provider>
  );
}

export function useSearchPalette() {
  return useContext(SearchPaletteContext);
}
