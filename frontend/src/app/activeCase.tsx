import { createContext, useCallback, useContext, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { addCaseItem, createCase, deleteCaseItem, listCases } from "../lib/api";
import type { Case, CaseItem, CaseItemKind } from "../lib/types";
import { pluralize } from "../lib/format";
import { Dialog } from "../ui/Dialog";
import { usePreference } from "./preferences";
import { useToast } from "./toast";

export interface CaseItemDraft {
  kind: CaseItemKind;
  ref: string;
  note?: string;
}

interface ActiveCaseValue {
  cases: Case[];
  activeCase: Case | null;
  setActiveCaseID: (id: number | null) => void;
  // addToCase pins items to the active case, asking which case first when
  // none is active. label names the items in the confirmation ("thor1…a2f").
  addToCase: (items: CaseItemDraft[], label: string) => void;
  chooseCase: () => void;
}

const ActiveCaseContext = createContext<ActiveCaseValue>({
  cases: [],
  activeCase: null,
  setActiveCaseID: () => undefined,
  addToCase: () => undefined,
  chooseCase: () => undefined,
});

interface PendingPick {
  items: CaseItemDraft[];
  label: string;
}

export function ActiveCaseProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const casesQuery = useQuery({ queryKey: ["cases"], queryFn: listCases });
  const [activeCaseID, setActiveCaseID] = usePreference<number | null>("chain-analysis.active-case", null);
  const [pending, setPending] = useState<PendingPick | null>(null);
  const cases = useMemo(() => casesQuery.data ?? [], [casesQuery.data]);
  const activeCase = cases.find((item) => item.id === activeCaseID) ?? null;

  const pinItems = useCallback(
    async (target: Case, items: CaseItemDraft[], label: string) => {
      try {
        const created: CaseItem[] = [];
        for (const item of items) {
          created.push(await addCaseItem(target.id, item.kind, item.ref, item.note ?? ""));
        }
        await queryClient.invalidateQueries({ queryKey: ["cases"] });
        await queryClient.invalidateQueries({ queryKey: ["case", target.id] });
        toast(`Added ${label} to ${target.title}`, {
          actionLabel: "Undo",
          onAction: () => {
            void (async () => {
              for (const item of created) {
                await deleteCaseItem(target.id, item.id).catch(() => undefined);
              }
              await queryClient.invalidateQueries({ queryKey: ["cases"] });
              await queryClient.invalidateQueries({ queryKey: ["case", target.id] });
            })();
          },
        });
      } catch (error) {
        toast(error instanceof Error ? `Could not add to ${target.title}: ${error.message}` : "Could not add to the case.", {
          tone: "error",
        });
      }
    },
    [queryClient, toast]
  );

  const addToCase = useCallback(
    (items: CaseItemDraft[], label: string) => {
      const usable = items.filter((item) => item.ref.trim());
      if (!usable.length) {
        return;
      }
      if (activeCase) {
        void pinItems(activeCase, usable, label);
        return;
      }
      setPending({ items: usable, label });
    },
    [activeCase, pinItems]
  );

  const value = useMemo<ActiveCaseValue>(
    () => ({
      cases,
      activeCase,
      setActiveCaseID,
      addToCase,
      chooseCase: () => setPending({ items: [], label: "" }),
    }),
    [activeCase, addToCase, cases, setActiveCaseID]
  );

  return (
    <ActiveCaseContext.Provider value={value}>
      {children}
      {pending ? (
        <CasePickerDialog
          cases={cases}
          initialID={activeCase?.id ?? null}
          pending={pending}
          onClose={() => setPending(null)}
          onPick={async (target) => {
            setActiveCaseID(target.id);
            setPending(null);
            if (pending.items.length) {
              await pinItems(target, pending.items, pending.label);
            } else {
              toast(`Working on ${target.title}`);
            }
          }}
          onCreate={async (title) => {
            const created = await createCase(title);
            await queryClient.invalidateQueries({ queryKey: ["cases"] });
            return created;
          }}
        />
      ) : null}
    </ActiveCaseContext.Provider>
  );
}

export function useActiveCase() {
  return useContext(ActiveCaseContext);
}

function CasePickerDialog({
  cases,
  initialID,
  pending,
  onClose,
  onPick,
  onCreate,
}: {
  cases: Case[];
  initialID: number | null;
  pending: PendingPick;
  onClose: () => void;
  onPick: (target: Case) => Promise<void>;
  onCreate: (title: string) => Promise<Case>;
}) {
  const [selectedID, setSelectedID] = useState<number | null>(initialID ?? cases[0]?.id ?? null);
  const [newTitle, setNewTitle] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const adding = pending.items.length > 0;

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setBusy(true);
    try {
      if (newTitle.trim()) {
        const created = await onCreate(newTitle.trim());
        await onPick(created);
        return;
      }
      const target = cases.find((item) => item.id === selectedID);
      if (target) {
        await onPick(target);
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not create the case.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      title={adding ? `Add ${pending.label} to a case` : "Choose the case you're working on"}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            form="case-picker-form"
            className="btn btn-primary"
            disabled={busy || (!newTitle.trim() && selectedID === null)}
          >
            {adding ? "Add to case" : "Work on this case"}
          </button>
        </>
      }
    >
      <form id="case-picker-form" className="form-stack" onSubmit={(event) => void submit(event)}>
        <p>Findings you add from graphs, traces and actors go to the case you choose here.</p>
        {cases.length ? (
          <div className="ws-run-list" role="radiogroup" aria-label="Cases">
            {cases.map((item) => (
              <label key={item.id} className={`list-row is-interactive${selectedID === item.id && !newTitle.trim() ? " is-selected" : ""}`}>
                <input
                  type="radio"
                  name="case-picker"
                  checked={selectedID === item.id && !newTitle.trim()}
                  onChange={() => {
                    setSelectedID(item.id);
                    setNewTitle("");
                  }}
                />
                <span className="list-row-main">
                  <span className="list-row-title">{item.title}</span>
                  <span className="list-row-meta">{pluralize(item.item_count, "item")}</span>
                </span>
              </label>
            ))}
          </div>
        ) : null}
        <label className="field">
          <span>{cases.length ? "Or start a new case" : "New case title"}</span>
          <input
            value={newTitle}
            onChange={(event) => setNewTitle(event.target.value)}
            placeholder="Bitget hack, 2026-09-28"
            autoFocus={!cases.length}
          />
        </label>
        {error ? <p className="form-error">{error}</p> : null}
      </form>
    </Dialog>
  );
}
