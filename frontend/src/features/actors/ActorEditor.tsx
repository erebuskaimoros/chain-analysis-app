import { useMemo, useState, type FormEvent } from "react";
import { CloseIcon, PlusIcon } from "../../app/icons";
import { parseActorAddressLines } from "../../lib/actors";
import { EVM_CHAINS, guessAddressChain, KNOWN_CHAINS } from "../../lib/identify";
import type { Actor, ActorAddressInput, AddressAnnotation } from "../../lib/types";
import { Field } from "../../ui/Field";

export interface ActorDraft {
  id: number | null;
  name: string;
  color: string;
  notes: string;
  addresses: ActorAddressInput[];
}

export const NEW_ACTOR: ActorDraft = { id: null, name: "", color: "#4ca3ff", notes: "", addresses: [] };

export function draftFromActor(actor: Actor): ActorDraft {
  return {
    id: actor.id,
    name: actor.name,
    color: actor.color || "#4ca3ff",
    notes: actor.notes || "",
    addresses: actor.addresses.map((address) => ({
      address: address.address,
      chain_hint: address.chain_hint,
      label: address.label,
    })),
  };
}

interface ActorEditorProps {
  initial: ActorDraft;
  annotations: AddressAnnotation[];
  saving: boolean;
  error: string;
  onSave: (draft: ActorDraft) => void;
  onCancel: () => void;
}

function chainNote(address: string, chainHint: string) {
  const guess = guessAddressChain(address);
  if (!address.trim() || chainHint) {
    return "";
  }
  if (!guess) {
    return "Unrecognised address format";
  }
  if (!guess.certain) {
    return `EVM address: pick ${EVM_CHAINS.join(", ")} if you know it`;
  }
  return `Looks like ${guess.chain}`;
}

// ActorEditor edits an actor's name, colour, notes and addresses. Each address
// is a row with its chain and label; pasting a list fills many rows at once.
export function ActorEditor({ initial, annotations, saving, error, onSave, onCancel }: ActorEditorProps) {
  const [draft, setDraft] = useState<ActorDraft>(() => ({
    ...initial,
    addresses: initial.addresses.length ? initial.addresses : [{ address: "", chain_hint: "", label: "" }],
  }));
  const [pasteOpen, setPasteOpen] = useState(false);
  const [pasteText, setPasteText] = useState("");
  const [pasteError, setPasteError] = useState("");
  const [labeledPick, setLabeledPick] = useState("");

  const labeled = useMemo(
    () =>
      annotations
        .filter((annotation) => annotation.kind === "label" && annotation.value.trim())
        .sort((left, right) => left.value.localeCompare(right.value)),
    [annotations]
  );

  function updateRow(index: number, patch: Partial<ActorAddressInput>) {
    setDraft((current) => ({
      ...current,
      addresses: current.addresses.map((row, rowIndex) => (rowIndex === index ? { ...row, ...patch } : row)),
    }));
  }

  function addRows(rows: ActorAddressInput[]) {
    setDraft((current) => {
      const kept = current.addresses.filter((row) => row.address.trim());
      const seen = new Set(kept.map((row) => row.address.trim().toLowerCase()));
      const fresh = rows.filter((row) => {
        const key = row.address.trim().toLowerCase();
        if (!key || seen.has(key)) {
          return false;
        }
        seen.add(key);
        return true;
      });
      return { ...current, addresses: [...kept, ...fresh] };
    });
  }

  function fillChainFromGuess(index: number) {
    const row = draft.addresses[index];
    const guess = guessAddressChain(row.address);
    if (!row.chain_hint && guess?.certain) {
      updateRow(index, { chain_hint: guess.chain });
    }
  }

  function applyPaste() {
    const parsed = parseActorAddressLines(pasteText, annotations);
    if (parsed.errors.length) {
      setPasteError(parsed.errors.join(" "));
      return;
    }
    addRows(
      parsed.addresses.map((row) => {
        const guess = guessAddressChain(row.address);
        return { ...row, chain_hint: row.chain_hint || (guess?.certain ? guess.chain : "") };
      })
    );
    setPasteText("");
    setPasteError("");
    setPasteOpen(false);
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    onSave({
      ...draft,
      addresses: draft.addresses
        .map((row) => ({ address: row.address.trim(), chain_hint: row.chain_hint.trim().toUpperCase(), label: row.label.trim() }))
        .filter((row) => row.address),
    });
  }

  return (
    <form className="form-stack actor-editor" onSubmit={submit} aria-label={draft.id ? "Edit actor" : "New actor"}>
      <div className="actor-editor-top">
        <label className="field">
          <span>Name</span>
          <input
            value={draft.name}
            autoFocus={!draft.id}
            onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
            placeholder="Treasury cluster"
          />
        </label>
        <label className="field">
          <span>Colour</span>
          <input
            type="color"
            value={draft.color}
            onChange={(event) => setDraft((current) => ({ ...current, color: event.target.value }))}
          />
        </label>
      </div>
      <label className="field">
        <span>Notes</span>
        <textarea
          rows={3}
          value={draft.notes}
          onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
          placeholder="Who this is and why it matters."
        />
      </label>

      <div className="section">
        <div className="section-head">
          <h3>
            Addresses <span className="count">{draft.addresses.filter((row) => row.address.trim()).length}</span>
          </h3>
          <div className="form-actions">
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setPasteOpen((value) => !value)}>
              Paste a list
            </button>
            <button
              type="button"
              className="btn btn-sm"
              onClick={() =>
                setDraft((current) => ({ ...current, addresses: [...current.addresses, { address: "", chain_hint: "", label: "" }] }))
              }
            >
              <PlusIcon />
              Add address
            </button>
          </div>
        </div>

        {pasteOpen ? (
          <div className="paste-box">
            <Field
              label="One address per line"
              hint="Each line can be an address, address and label, address, chain and label, or the name of a labeled address."
            >
              <textarea
                className="mono"
                rows={5}
                value={pasteText}
                onChange={(event) => setPasteText(event.target.value)}
                placeholder={"thor1...,THOR,treasury hot\n0xabc...,ETH,ops signer\nTreasury Hot Wallet"}
              />
            </Field>
            {pasteError ? <p className="form-error">{pasteError}</p> : null}
            <div className="form-actions">
              <button type="button" className="btn btn-sm btn-primary" disabled={!pasteText.trim()} onClick={applyPaste}>
                Add these addresses
              </button>
              <button type="button" className="btn btn-sm" onClick={() => setPasteOpen(false)}>
                Cancel
              </button>
            </div>
          </div>
        ) : null}

        <div className="address-rows" role="table" aria-label="Addresses">
          <div className="address-row address-row-head" role="row">
            <span role="columnheader">Address</span>
            <span role="columnheader">Chain</span>
            <span role="columnheader">Label</span>
            <span role="columnheader" className="visually-hidden">
              Remove
            </span>
          </div>
          {draft.addresses.map((row, index) => {
            const note = chainNote(row.address, row.chain_hint);
            return (
              <div className="address-row" role="row" key={index}>
                <div role="cell" className="address-cell">
                  <input
                    className="input mono"
                    aria-label={`Address ${index + 1}`}
                    value={row.address}
                    placeholder="thor1…, 0x…, bc1…"
                    spellCheck={false}
                    onChange={(event) => updateRow(index, { address: event.target.value })}
                    onBlur={() => fillChainFromGuess(index)}
                  />
                  {note ? <small className="field-hint">{note}</small> : null}
                </div>
                <div role="cell">
                  <select
                    className="select"
                    aria-label={`Chain for address ${index + 1}`}
                    value={row.chain_hint}
                    onChange={(event) => updateRow(index, { chain_hint: event.target.value })}
                  >
                    <option value="">Any chain</option>
                    {[...new Set([...KNOWN_CHAINS, row.chain_hint].filter(Boolean))].map((chain) => (
                      <option key={chain} value={chain}>
                        {chain}
                      </option>
                    ))}
                  </select>
                </div>
                <div role="cell">
                  <input
                    className="input"
                    aria-label={`Label for address ${index + 1}`}
                    value={row.label}
                    placeholder="Optional"
                    onChange={(event) => updateRow(index, { label: event.target.value })}
                  />
                </div>
                <div role="cell">
                  <button
                    type="button"
                    className="btn btn-ghost btn-icon btn-sm"
                    aria-label={`Remove address ${index + 1}`}
                    onClick={() =>
                      setDraft((current) => ({
                        ...current,
                        addresses: current.addresses.filter((_, rowIndex) => rowIndex !== index),
                      }))
                    }
                  >
                    <CloseIcon />
                  </button>
                </div>
              </div>
            );
          })}
        </div>

        {labeled.length ? (
          <div className="inline-form">
            <select
              className="select"
              aria-label="Add a labeled address"
              value={labeledPick}
              onChange={(event) => setLabeledPick(event.target.value)}
            >
              <option value="">Add one of your labeled addresses…</option>
              {labeled.map((annotation) => (
                <option key={annotation.id} value={String(annotation.id)}>
                  {annotation.value}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="btn"
              disabled={!labeledPick}
              onClick={() => {
                const annotation = labeled.find((item) => String(item.id) === labeledPick);
                if (annotation) {
                  const guess = guessAddressChain(annotation.address);
                  addRows([
                    { address: annotation.address, chain_hint: guess?.certain ? guess.chain : "", label: annotation.value },
                  ]);
                }
                setLabeledPick("");
              }}
            >
              Add
            </button>
          </div>
        ) : null}
      </div>

      {error ? <p className="form-error">{error}</p> : null}
      <div className="form-actions">
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? "Saving…" : draft.id ? "Save changes" : "Create actor"}
        </button>
        <button type="button" className="btn" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}
