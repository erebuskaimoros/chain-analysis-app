import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createActor, deleteActor, listActors, updateActor } from "../../lib/api";
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
  const [draft, setDraft] = useState<ActorDraft>(defaultDraft);
  const [formError, setFormError] = useState("");

  const saveMutation = useMutation({
    mutationFn: async (value: ActorDraft) => {
      const parsed = parseActorAddressLines(value.addressesText);
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
              placeholder={"thor1...,THOR,treasury hot\n0xabc...,ETH,ops signer"}
            />
            <small>One address per line. Format: `address`, `address,label`, or `address,CHAIN,label`.</small>
          </label>
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
