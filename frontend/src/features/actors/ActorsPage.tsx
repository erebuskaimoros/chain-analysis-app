import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createActor, deleteActor, listActors, listAnnotations, updateActor } from "../../lib/api";
import { formatActorAddressLines, parseActorAddressLines } from "../../lib/actors";
import type { Actor } from "../../lib/types";

interface ActorDraft {
  id: number | null;
  name: string;
  color: string;
  notes: string;
  addressesText: string;
}

const defaultDraft: ActorDraft = {
  id: null,
  name: "",
  color: "#4ca3ff",
  notes: "",
  addressesText: "",
};

export function ActorsPage() {
  const queryClient = useQueryClient();
  const actorsQuery = useQuery({
    queryKey: ["actors"],
    queryFn: listActors,
  });
  const annotationsQuery = useQuery({
    queryKey: ["annotations"],
    queryFn: listAnnotations,
  });
  const [draft, setDraft] = useState<ActorDraft>(defaultDraft);
  const [formError, setFormError] = useState("");
  const [selectedNamedAddressID, setSelectedNamedAddressID] = useState("");

  const namedAddresses = useMemo(
    () =>
      [...(annotationsQuery.data ?? [])]
        .filter((annotation) => annotation.kind === "label" && annotation.value.trim())
        .sort(
          (left, right) =>
            left.value.localeCompare(right.value) || left.address.localeCompare(right.address)
        ),
    [annotationsQuery.data]
  );

  const saveMutation = useMutation({
    mutationFn: async (value: ActorDraft) => {
      const parsed = parseActorAddressLines(value.addressesText, namedAddresses);
      if (!value.name.trim()) {
        throw new Error("Actor name is required.");
      }
      if (parsed.errors.length) {
        throw new Error(parsed.errors.join(" · "));
      }

      const payload = {
        name: value.name.trim(),
        color: value.color.trim() || "#4ca3ff",
        notes: value.notes.trim(),
        addresses: parsed.addresses,
      };

      if (value.id) {
        return updateActor(value.id, payload);
      }

      return createActor(payload);
    },
    onSuccess: async () => {
      setDraft(defaultDraft);
      setFormError("");
      await queryClient.invalidateQueries({ queryKey: ["actors"] });
    },
    onError: (error) => {
      setFormError(error instanceof Error ? error.message : "Unable to save actor.");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteActor,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["actors"] });
    },
  });

  const sortedActors = useMemo(
    () => [...(actorsQuery.data ?? [])].sort((left, right) => left.name.localeCompare(right.name)),
    [actorsQuery.data]
  );

  function startEdit(actor: Actor) {
    setDraft({
      id: actor.id,
      name: actor.name,
      color: actor.color || "#4ca3ff",
      notes: actor.notes || "",
      addressesText: formatActorAddressLines(actor.addresses),
    });
    setFormError("");
  }

  function resetForm() {
    setDraft(defaultDraft);
    setFormError("");
    setSelectedNamedAddressID("");
  }

  async function handleDelete(actor: Actor) {
    if (!window.confirm(`Delete actor "${actor.name}"?`)) {
      return;
    }
    await deleteMutation.mutateAsync(actor.id);
    if (draft.id === actor.id) {
      resetForm();
    }
  }

  function addNamedAddress() {
    const annotation = namedAddresses.find((item) => String(item.id) === selectedNamedAddressID);
    if (!annotation) {
      return;
    }
    const line = annotation.value;
    setDraft((current) => ({
      ...current,
      addressesText: current.addressesText.trim() ? `${current.addressesText.trim()}\n${line}` : line,
    }));
    setSelectedNamedAddressID("");
    setFormError("");
  }

  return (
    <div className="page-grid two-up">
      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Actors</span>
            <h2>{draft.id ? "Edit Actor" : "Create Actor"}</h2>
          </div>
          {draft.id ? (
            <button type="button" className="button secondary" onClick={resetForm}>
              New Actor
            </button>
          ) : null}
        </div>
        <form
          className="form-grid"
          onSubmit={(event) => {
            event.preventDefault();
            saveMutation.mutate(draft);
          }}
        >
          <label className="field">
            <span>Name</span>
            <input
              value={draft.name}
              onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
              placeholder="Treasury cluster"
            />
          </label>
          <label className="field">
            <span>Color</span>
            <input
              type="color"
              value={draft.color}
              onChange={(event) => setDraft((current) => ({ ...current, color: event.target.value }))}
            />
          </label>
          <label className="field field-full">
            <span>Notes</span>
            <textarea
              rows={4}
              value={draft.notes}
              onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
              placeholder="Optional context for this actor."
            />
          </label>
          <label className="field field-full">
            <span>Addresses</span>
            <textarea
              rows={10}
              value={draft.addressesText}
              onChange={(event) => setDraft((current) => ({ ...current, addressesText: event.target.value }))}
              placeholder={"thor1...,THOR,treasury hot\n0xabc...,ETH,ops signer\nTreasury Hot Wallet"}
            />
            <small>
              One address per line. Format: `address`, `address,label`, `address,CHAIN,label`, or a saved label
              annotation name.
            </small>
          </label>
          <div className="field field-full">
            <span>Named Addresses</span>
            <div className="button-row">
              <select
                value={selectedNamedAddressID}
                onChange={(event) => setSelectedNamedAddressID(event.target.value)}
                disabled={annotationsQuery.isLoading || !namedAddresses.length}
              >
                <option value="">Select a saved label annotation</option>
                {namedAddresses.map((annotation) => (
                  <option key={annotation.id} value={String(annotation.id)}>
                    {annotation.value} · {annotation.address}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="button secondary"
                onClick={addNamedAddress}
                disabled={!selectedNamedAddressID}
              >
                Add Named Address
              </button>
            </div>
            <small>
              Saved `label` annotations appear here. Adding one inserts its name into the actor field and resolves it
              to the underlying address on save.
            </small>
          </div>
          {formError ? <p className="error-text form-message">{formError}</p> : null}
          <div className="form-actions field-full">
            <button type="submit" className="button" disabled={saveMutation.isPending}>
              {saveMutation.isPending ? "Saving..." : draft.id ? "Update Actor" : "Save Actor"}
            </button>
            <button type="button" className="button secondary" onClick={resetForm}>
              Reset
            </button>
          </div>
        </form>
      </section>

      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Directory</span>
            <h2>Saved Actors</h2>
          </div>
          <span className="status-pill ok">{sortedActors.length}</span>
        </div>
        {actorsQuery.isLoading ? <div className="empty-state">Loading actors…</div> : null}
        {actorsQuery.error ? <p className="error-text">{actorsQuery.error.message}</p> : null}
        {!actorsQuery.isLoading && !sortedActors.length ? (
          <div className="empty-state">Create an actor to start graphing flows.</div>
        ) : null}
        <div className="card-list">
          {sortedActors.map((actor) => (
            <article key={actor.id} className="entity-card">
              <div className="entity-card-head">
                <div className="entity-title">
                  <span className="actor-color-swatch" style={{ background: actor.color || "#4ca3ff" }} />
                  <strong>{actor.name}</strong>
                </div>
                <span className="badge">{actor.addresses.length}</span>
              </div>
              <p className="entity-note">{actor.notes || "No notes"}</p>
              <div className="entity-address-list mono-wrap">
                {actor.addresses.slice(0, 4).map((address) => (
                  <div key={address.id}>
                    {address.address}
                    {address.label ? ` · ${address.label}` : ""}
                  </div>
                ))}
                {actor.addresses.length > 4 ? <div>+{actor.addresses.length - 4} more</div> : null}
              </div>
              <div className="button-row">
                <button type="button" className="button secondary" onClick={() => startEdit(actor)}>
                  Edit
                </button>
                <button
                  type="button"
                  className="button secondary danger"
                  disabled={deleteMutation.isPending}
                  onClick={() => {
                    void handleDelete(actor);
                  }}
                >
                  Delete
                </button>
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}
