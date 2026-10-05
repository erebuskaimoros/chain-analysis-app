import { useEffect, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { addCaseItem, caseExportURL, createCase, deleteCase, deleteCaseItem, getCase, listCases, updateCase } from "../../lib/api";
import { formatShortDateTime } from "../../lib/format";
import { explorerURLForAddress, explorerURLForTx } from "../../lib/graph/actions";
import type { CaseItem, CaseItemKind } from "../../lib/types";

const ITEM_KINDS: Array<{ kind: CaseItemKind; label: string; placeholder: string }> = [
  { kind: "address", label: "Address", placeholder: "ETH|0x… or thor1…" },
  { kind: "tx", label: "Transaction", placeholder: "Transaction hash" },
  { kind: "trace_run", label: "Saved trace", placeholder: "Trace ID" },
  { kind: "graph_state", label: "Graph state", placeholder: "Graph state ID" },
  { kind: "actor", label: "Actor", placeholder: "Actor ID" },
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

export function CasesPage() {
  const queryClient = useQueryClient();
  const casesQuery = useQuery({ queryKey: ["cases"], queryFn: listCases });
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const caseQuery = useQuery({ queryKey: ["case", selectedID], queryFn: () => getCase(selectedID as number), enabled: selectedID != null });
  const [newTitle, setNewTitle] = useState("");
  const [title, setTitle] = useState("");
  const [notes, setNotes] = useState("");
  const [itemKind, setItemKind] = useState<CaseItemKind>("address");
  const [itemRef, setItemRef] = useState("");
  const [itemNote, setItemNote] = useState("");
  const [status, setStatus] = useState("");
  const current = caseQuery.data;

  useEffect(() => {
    if (current) {
      setTitle(current.title);
      setNotes(current.notes_md);
    }
  }, [current]);

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
      setSelectedID(created.id);
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

  return (
    <div className="page-stack">
      <div className="page-grid two-up">
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Investigations</span>
              <h2>Cases</h2>
            </div>
          </div>
          <form className="form-grid" onSubmit={(event) => void onCreate(event)}>
            <label className="field field-full">
              <span>New case title</span>
              <input value={newTitle} onChange={(event) => setNewTitle(event.target.value)} placeholder="Bitget hack, 2026-09-28" />
            </label>
            <div className="button-row field-full">
              <button type="submit" className="button" disabled={!newTitle.trim()}>
                Create case
              </button>
            </div>
          </form>
          {casesQuery.data?.length ? (
            <div className="card-list">
              {casesQuery.data.map((item) => (
                <article key={item.id} className={`entity-card${item.id === selectedID ? " selected" : ""}`}>
                  <strong>{item.title}</strong>
                  <p>
                    {item.item_count} items · updated {formatShortDateTime(item.updated_at)}
                  </p>
                  <div className="button-row">
                    <button type="button" className="button secondary" aria-pressed={item.id === selectedID} onClick={() => setSelectedID(item.id)}>
                      Open
                    </button>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <p className="empty-state">No cases yet.</p>
          )}
          {status ? <p className="form-message">{status}</p> : null}
        </section>

        <section className="panel page-panel" aria-label="Case details">
          {current ? (
            <>
              <div className="panel-head">
                <div>
                  <span className="eyebrow">Case {current.id}</span>
                  <h2>{current.title}</h2>
                </div>
                <div className="button-row">
                  <a className="button secondary" href={caseExportURL(current.id, "md")} download>
                    Export Markdown
                  </a>
                  <a className="button secondary" href={caseExportURL(current.id, "csv")} download>
                    Export flows CSV
                  </a>
                </div>
              </div>
              <form
                className="form-grid"
                onSubmit={(event) => {
                  event.preventDefault();
                  void run(() => updateCase(current.id, title, notes), "Saved.");
                }}
              >
                <label className="field field-full">
                  <span>Title</span>
                  <input value={title} onChange={(event) => setTitle(event.target.value)} />
                </label>
                <label className="field field-full">
                  <span>Notes (Markdown)</span>
                  <textarea rows={5} value={notes} onChange={(event) => setNotes(event.target.value)} />
                </label>
                <div className="button-row field-full">
                  <button type="submit" className="button">
                    Save notes
                  </button>
                  <button
                    type="button"
                    className="button secondary"
                    onClick={() =>
                      void run(async () => {
                        await deleteCase(current.id);
                        setSelectedID(null);
                      }, "Case deleted.")
                    }
                  >
                    Delete case
                  </button>
                </div>
              </form>

              <h3>Items</h3>
              {current.items?.length ? (
                <div className="table-wrap compact">
                  <table className="data-table compact">
                    <thead>
                      <tr>
                        <th>Kind</th>
                        <th>Item</th>
                        <th>Note</th>
                        <th />
                      </tr>
                    </thead>
                    <tbody>
                      {current.items.map((item) => {
                        const link = itemLink(item);
                        return (
                          <tr key={item.id}>
                            <td>{item.kind.replace("_", " ")}</td>
                            <td className="mono-wrap">
                              {link.url ? (
                                <a className="table-link" href={link.url} target="_blank" rel="noreferrer">
                                  {link.text}
                                </a>
                              ) : (
                                link.text
                              )}
                            </td>
                            <td>{item.note}</td>
                            <td>
                              <button
                                type="button"
                                className="button secondary small"
                                onClick={() => void run(() => deleteCaseItem(current.id, item.id), "Item removed.")}
                              >
                                Remove
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="empty-state">Pin addresses, transactions, saved traces, graph states or actors to this case.</p>
              )}

              <form className="form-grid" onSubmit={(event) => void onAddItem(event)}>
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
                <label className="field field-full">
                  <span>Note</span>
                  <input value={itemNote} onChange={(event) => setItemNote(event.target.value)} />
                </label>
                <div className="button-row field-full">
                  <button type="submit" className="button" disabled={!itemRef.trim()}>
                    Pin item
                  </button>
                </div>
              </form>
            </>
          ) : (
            <p className="empty-state">Open a case to see its items, notes and exports.</p>
          )}
        </section>
      </div>
    </div>
  );
}
