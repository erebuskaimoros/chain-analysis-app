import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { DownloadIcon, ExternalIcon, PlusIcon, TrashIcon } from "../../app/icons";
import { useActiveCase } from "../../app/activeCase";
import { useRouteIntent, useRouter, type ViewKey } from "../../app/router";
import { useToast } from "../../app/toast";
import { addCaseItem, caseExportURL, createCase, deleteCase, deleteCaseItem, getCase, listCases, updateCase } from "../../lib/api";
import { formatRelativeTime, middleTruncate, pluralize } from "../../lib/format";
import { explorerURLForAddress, explorerURLForTx } from "../../lib/graph/actions";
import type { CaseItem, CaseItemKind } from "../../lib/types";
import { ConfirmDialog } from "../../ui/Dialog";
import { MenuButton } from "../../ui/Menu";
import { PageHeader } from "../../ui/PageHeader";

const ITEM_KINDS: Array<{ kind: CaseItemKind; label: string; plural: string; placeholder: string }> = [
  { kind: "address", label: "Address", plural: "Addresses", placeholder: "ETH|0x… or thor1…" },
  { kind: "tx", label: "Transaction", plural: "Transactions", placeholder: "Transaction hash" },
  { kind: "trace_run", label: "Saved trace", plural: "Saved traces", placeholder: "Trace ID" },
  { kind: "graph_state", label: "Graph state", plural: "Saved graphs", placeholder: "Graph state ID" },
  { kind: "actor", label: "Actor", plural: "Actors", placeholder: "Actor ID" },
];

function itemLink(item: CaseItem) {
  if (item.kind === "address") {
    const [chain, address] = item.ref.includes("|") ? item.ref.split("|", 2) : ["", item.ref];
    return { text: item.title ? `${item.title} (${address})` : item.ref, url: explorerURLForAddress(address, chain) };
  }
  if (item.kind === "tx") {
    return { text: item.ref, url: explorerURLForTx(item.ref, "THOR") };
  }
  const kindLabel = ITEM_KINDS.find((entry) => entry.kind === item.kind)?.label ?? item.kind;
  return { text: `${kindLabel} ${item.ref}${item.title ? `: ${item.title}` : ""}`, url: "" };
}

// Where an item opens inside the app.
function itemTarget(item: CaseItem): { view: ViewKey; params: Record<string, string>; label: string } | null {
  switch (item.kind) {
    case "address": {
      const address = item.ref.includes("|") ? item.ref.split("|", 2)[1] : item.ref;
      return { view: "explorer", params: { address }, label: "Explore" };
    }
    case "tx":
      return { view: "explorer", params: { tx: item.ref }, label: "Look up" };
    case "trace_run":
      return { view: "trace", params: { trace: item.ref }, label: "Open trace" };
    case "graph_state":
      return { view: "graph", params: { state: item.ref }, label: "Open graph" };
    case "actor":
      return { view: "actors", params: { actor: item.ref }, label: "Open actor" };
    default:
      return null;
  }
}

export function CasesPage() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { navigate } = useRouter();
  const { activeCase, setActiveCaseID } = useActiveCase();
  const casesQuery = useQuery({ queryKey: ["cases"], queryFn: listCases });
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const caseQuery = useQuery({ queryKey: ["case", selectedID], queryFn: () => getCase(selectedID as number), enabled: selectedID != null });
  const [creating, setCreating] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [title, setTitle] = useState("");
  const [notes, setNotes] = useState("");
  const [itemKind, setItemKind] = useState<CaseItemKind>("address");
  const [itemRef, setItemRef] = useState("");
  const [itemNote, setItemNote] = useState("");
  const [status, setStatus] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const current = caseQuery.data;
  const cases = useMemo(() => casesQuery.data ?? [], [casesQuery.data]);

  useEffect(() => {
    if (current) {
      setTitle(current.title);
      setNotes(current.notes_md);
    }
  }, [current]);

  useRouteIntent("cases", (params) => {
    if (params.case) {
      setSelectedID(Number(params.case));
    }
    if (params.new === "1") {
      setCreating(true);
    }
  });

  async function refresh() {
    await queryClient.invalidateQueries({ queryKey: ["cases"] });
    await queryClient.invalidateQueries({ queryKey: ["case", selectedID] });
  }

  async function run(action: () => Promise<unknown>, done: string) {
    try {
      await action();
      setStatus(done);
      await refresh();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Request failed.");
    }
  }

  async function onCreate(event: FormEvent) {
    event.preventDefault();
    try {
      const created = await createCase(newTitle);
      setNewTitle("");
      setCreating(false);
      setSelectedID(created.id);
      if (!activeCase) {
        setActiveCaseID(created.id);
      }
      setStatus(`Created "${created.title}".`);
      await queryClient.invalidateQueries({ queryKey: ["cases"] });
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Could not create the case.");
    }
  }

  async function onAddItem(event: FormEvent) {
    event.preventDefault();
    if (selectedID == null) {
      return;
    }
    await run(async () => {
      await addCaseItem(selectedID, itemKind, itemRef, itemNote);
      setItemRef("");
      setItemNote("");
    }, "Item pinned.");
  }

  const placeholder = ITEM_KINDS.find((entry) => entry.kind === itemKind)?.placeholder ?? "";
  const items = current?.items ?? [];
  const isActive = Boolean(current && activeCase?.id === current.id);

  const createForm = (
    <form className="split-list-new" onSubmit={(event) => void onCreate(event)}>
      <label className="visually-hidden" htmlFor="new-case-title">
        New case title
      </label>
      <input
        id="new-case-title"
        className="input"
        autoFocus={creating}
        value={newTitle}
        onChange={(event) => setNewTitle(event.target.value)}
        placeholder="Bitget hack, 2026-09-28"
      />
      <button type="submit" className="btn btn-primary" disabled={!newTitle.trim()}>
        Create case
      </button>
    </form>
  );

  return (
    <>
      <PageHeader
        title="Cases"
        context={cases.length ? pluralize(cases.length, "case") : ""}
        actions={
          <button type="button" className="btn btn-sm btn-primary" onClick={() => setCreating(true)}>
            <PlusIcon />
            New case
          </button>
        }
      />
      <div className="split">
        <aside className="split-list" aria-label="Cases">
          {creating || !cases.length ? createForm : null}
          <ul className="list">
            {cases.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  className={`list-row${item.id === selectedID ? " is-selected" : ""}`}
                  aria-current={item.id === selectedID ? "true" : undefined}
                  onClick={() => setSelectedID(item.id)}
                >
                  <span className="list-row-main">
                    <span className="list-row-title">{item.title}</span>
                    <span className="list-row-meta">
                      {pluralize(item.item_count, "item")}, updated {formatRelativeTime(item.updated_at)}
                    </span>
                  </span>
                  {activeCase?.id === item.id ? <span className="chip chip-accent">Active</span> : null}
                </button>
              </li>
            ))}
          </ul>
          {status ? <p className="status-text split-pad">{status}</p> : null}
        </aside>

        <section className="split-detail" aria-label="Case details">
          {current ? (
            <>
              <div className="actor-detail-head">
                <div>
                  <h2 className="split-detail-title">{current.title}</h2>
                  <p className="section-note">
                    {pluralize(items.length, "item")}, updated {formatRelativeTime(current.updated_at)}
                  </p>
                </div>
                <div className="form-actions">
                  {isActive ? (
                    <span className="chip chip-accent">Active case</span>
                  ) : (
                    <button type="button" className="btn btn-sm btn-primary" onClick={() => setActiveCaseID(current.id)}>
                      Work on this case
                    </button>
                  )}
                  <a className="btn btn-sm" href={caseExportURL(current.id, "md")} download>
                    <DownloadIcon />
                    Export Markdown
                  </a>
                  <a className="btn btn-sm" href={caseExportURL(current.id, "csv")} download>
                    <DownloadIcon />
                    Export flows CSV
                  </a>
                  <MenuButton
                    label={`More actions for ${current.title}`}
                    className="btn btn-sm btn-icon"
                    items={[{ label: "Delete case…", icon: <TrashIcon />, danger: true, onSelect: () => setConfirmDelete(true) }]}
                  />
                </div>
              </div>

              <form
                className="form-stack"
                onSubmit={(event) => {
                  event.preventDefault();
                  void run(() => updateCase(current.id, title, notes), "Saved.");
                }}
              >
                <label className="field">
                  <span>Title</span>
                  <input value={title} onChange={(event) => setTitle(event.target.value)} />
                </label>
                <label className="field">
                  <span>Notes (Markdown)</span>
                  <textarea rows={6} value={notes} onChange={(event) => setNotes(event.target.value)} />
                </label>
                <div className="form-actions">
                  <button
                    type="submit"
                    className="btn btn-primary btn-sm"
                    disabled={title === current.title && notes === current.notes_md}
                  >
                    Save notes
                  </button>
                </div>
              </form>

              <section className="section" aria-label="Items">
                <h3>
                  Items <span className="count">{items.length}</span>
                </h3>
                {items.length ? (
                  ITEM_KINDS.filter((entry) => items.some((item) => item.kind === entry.kind)).map((entry) => (
                    <div key={entry.kind} className="address-group">
                      <div className="address-group-head section-title">{entry.plural}</div>
                      <ul className="list ws-run-list">
                        {items
                          .filter((item) => item.kind === entry.kind)
                          .map((item) => {
                            const link = itemLink(item);
                            const target = itemTarget(item);
                            return (
                              <li key={item.id} className="list-row">
                                <span className="list-row-main">
                                  <span className={`list-row-title${item.kind === "address" || item.kind === "tx" ? " mono" : ""}`} title={item.ref}>
                                    {item.kind === "tx" ? middleTruncate(link.text, 12, 8) : link.text}
                                  </span>
                                  {item.note ? <span className="list-row-meta">{item.note}</span> : null}
                                </span>
                                <span className="list-row-side">{formatRelativeTime(item.pinned_at)}</span>
                                {target ? (
                                  <button type="button" className="btn btn-sm" onClick={() => navigate(target.view, target.params)}>
                                    {target.label}
                                  </button>
                                ) : null}
                                {link.url ? (
                                  <a
                                    className="btn btn-ghost btn-icon btn-sm"
                                    href={link.url}
                                    target="_blank"
                                    rel="noreferrer"
                                    title="Open in a block explorer"
                                    aria-label={`Open ${link.text} in a block explorer`}
                                  >
                                    <ExternalIcon />
                                  </a>
                                ) : null}
                                <button
                                  type="button"
                                  className="btn btn-ghost btn-sm"
                                  onClick={() => void run(() => deleteCaseItem(current.id, item.id), "Item removed.")}
                                >
                                  Remove
                                </button>
                              </li>
                            );
                          })}
                      </ul>
                    </div>
                  ))
                ) : (
                  <div className="empty-state">
                    <strong>Nothing pinned yet</strong>
                    {isActive
                      ? "This is the active case. Use Add to case on graph nodes, transactions, trace results and actors to file findings here."
                      : "Make this the active case, then use Add to case on graph nodes, transactions, trace results and actors to file findings here."}
                  </div>
                )}

                <details className="disclosure">
                  <summary>Add an item by reference</summary>
                  <form className="disclosure-body" onSubmit={(event) => void onAddItem(event)}>
                    <div className="field-row">
                      <label className="field">
                        <span>Pin</span>
                        <select value={itemKind} onChange={(event) => setItemKind(event.target.value as CaseItemKind)}>
                          {ITEM_KINDS.map((entry) => (
                            <option key={entry.kind} value={entry.kind}>
                              {entry.label}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="field">
                        <span>Reference</span>
                        <input value={itemRef} onChange={(event) => setItemRef(event.target.value)} placeholder={placeholder} />
                      </label>
                    </div>
                    <label className="field">
                      <span>Note</span>
                      <input value={itemNote} onChange={(event) => setItemNote(event.target.value)} placeholder="Optional" />
                    </label>
                    <div className="form-actions">
                      <button type="submit" className="btn btn-sm" disabled={!itemRef.trim()}>
                        Pin item
                      </button>
                    </div>
                  </form>
                </details>
              </section>
            </>
          ) : selectedID != null && caseQuery.isLoading ? (
            <p className="status-text">Loading the case…</p>
          ) : cases.length ? (
            <p className="empty-state">Pick a case on the left to see its notes, items and exports.</p>
          ) : !casesQuery.isLoading ? (
            <div className="empty-state">
              <strong>Start your first case</strong>
              <span>
                A case collects the addresses, transactions, traces, graphs and actors behind a finding, with your notes, and
                exports them as a Markdown report or a flows CSV.
              </span>
              <span>
                Name it on the left. While it is the active case, the Add to case buttons on graphs, traces and actors file
                findings straight into it.
              </span>
            </div>
          ) : null}
        </section>
      </div>

      {confirmDelete && current ? (
        <ConfirmDialog
          title={`Delete ${current.title}?`}
          message={`This deletes the case, its notes and its ${pluralize(items.length, "pinned item")}. The addresses, traces and graphs themselves are not deleted.`}
          confirmLabel="Delete case"
          onCancel={() => setConfirmDelete(false)}
          onConfirm={() => {
            const target = current;
            setConfirmDelete(false);
            void run(async () => {
              await deleteCase(target.id);
              if (activeCase?.id === target.id) {
                setActiveCaseID(null);
              }
              setSelectedID(null);
            }, "Case deleted.").then(() => toast(`Deleted ${target.title}`));
          }}
        />
      ) : null}
    </>
  );
}
