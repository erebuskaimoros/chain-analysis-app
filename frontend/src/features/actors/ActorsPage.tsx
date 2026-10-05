import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { EditIcon, ExplorerIcon, GraphIcon, PinIcon, PlusIcon, SearchIcon, TraceIcon, TrashIcon } from "../../app/icons";
import { useActiveCase } from "../../app/activeCase";
import { useRouteIntent, useRouter } from "../../app/router";
import { useToast } from "../../app/toast";
import { createActor, deleteActor, getActorMonitor, listActors, listAnnotations, updateActor } from "../../lib/api";
import { formatCompactUSD, middleTruncate, pluralize } from "../../lib/format";
import { encodeSeed, guessAddressChain } from "../../lib/identify";
import type { Actor, ActorAddress } from "../../lib/types";
import { AddressText, ChainBadge } from "../../ui/AddressText";
import { ConfirmDialog } from "../../ui/Dialog";
import { MenuButton } from "../../ui/Menu";
import { PageHeader } from "../../ui/PageHeader";
import { ActorEditor, draftFromActor, NEW_ACTOR, type ActorDraft } from "./ActorEditor";
import { ActorMonitorPanel } from "./ActorMonitorPanel";

type Mode = { kind: "view" } | { kind: "new" } | { kind: "edit"; actorID: number };

function chainCounts(actor: Actor) {
  const counts = new Map<string, number>();
  actor.addresses.forEach((address) => {
    const chain = (address.chain_hint || guessAddressChain(address.address)?.chain || "?").toUpperCase();
    counts.set(chain, (counts.get(chain) ?? 0) + 1);
  });
  return Array.from(counts.entries()).sort((left, right) => right[1] - left[1]);
}

// Addresses with no chain set whose format names an external chain. Live
// holdings need the chain to look these up.
function missingChainHints(actor: Actor) {
  return actor.addresses
    .map((address) => ({ address, guess: guessAddressChain(address.address) }))
    .filter(
      ({ address, guess }) => !address.chain_hint && guess?.certain && guess.chain !== "THOR" && guess.chain !== "MAYA"
    );
}

export function ActorsPage() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { navigate } = useRouter();
  const { addToCase } = useActiveCase();
  const actorsQuery = useQuery({ queryKey: ["actors"], queryFn: listActors });
  const annotationsQuery = useQuery({ queryKey: ["annotations"], queryFn: listAnnotations });
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [mode, setMode] = useState<Mode>({ kind: "view" });
  const [filter, setFilter] = useState("");
  const [formError, setFormError] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<Actor | null>(null);

  const actors = useMemo(
    () => [...(actorsQuery.data ?? [])].sort((left, right) => left.name.localeCompare(right.name)),
    [actorsQuery.data]
  );
  const visibleActors = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    if (!needle) {
      return actors;
    }
    return actors.filter(
      (actor) =>
        actor.name.toLowerCase().includes(needle) ||
        actor.addresses.some(
          (address) => address.address.toLowerCase().includes(needle) || address.label.toLowerCase().includes(needle)
        )
    );
  }, [actors, filter]);
  const selected = actors.find((actor) => actor.id === selectedID) ?? null;

  useEffect(() => {
    if (selectedID === null && actors.length && mode.kind === "view") {
      setSelectedID(actors[0].id);
    }
  }, [actors, mode.kind, selectedID]);

  useRouteIntent("actors", (params) => {
    if (params.actor) {
      setSelectedID(Number(params.actor));
      setMode({ kind: "view" });
    }
    if (params.new === "1") {
      setMode({ kind: "new" });
    }
  });

  const saveMutation = useMutation({
    mutationFn: async (draft: ActorDraft) => {
      if (!draft.name.trim()) {
        throw new Error("Give the actor a name.");
      }
      const payload = {
        name: draft.name.trim(),
        color: draft.color.trim() || "#4ca3ff",
        notes: draft.notes.trim(),
        addresses: draft.addresses,
      };
      return draft.id ? updateActor(draft.id, payload) : createActor(payload);
    },
    onSuccess: async (saved, draft) => {
      setFormError("");
      setMode({ kind: "view" });
      setSelectedID(saved?.id ?? draft.id ?? null);
      await queryClient.invalidateQueries({ queryKey: ["actors"] });
      toast(draft.id ? `Saved ${draft.name}` : `Created ${draft.name}`);
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

  async function applyChainHints(actor: Actor) {
    const hints = new Map(missingChainHints(actor).map(({ address, guess }) => [address.id, guess!.chain]));
    try {
      await updateActor(actor.id, {
        name: actor.name,
        color: actor.color,
        notes: actor.notes,
        addresses: actor.addresses.map((address) => ({
          address: address.address,
          chain_hint: hints.get(address.id) ?? address.chain_hint,
          label: address.label,
        })),
      });
      await queryClient.invalidateQueries({ queryKey: ["actors"] });
      toast(`Set the chain on ${pluralize(hints.size, "address", "addresses")}`);
    } catch (error) {
      toast(error instanceof Error ? error.message : "Could not update the actor.", { tone: "error" });
    }
  }

  const editing = mode.kind === "edit" ? actors.find((actor) => actor.id === mode.actorID) ?? null : null;

  return (
    <>
      <PageHeader
        title="Actors"
        context={actors.length ? pluralize(actors.length, "actor") : ""}
        actions={
          <button
            type="button"
            className="btn btn-sm btn-primary"
            onClick={() => {
              setFormError("");
              setMode({ kind: "new" });
            }}
          >
            <PlusIcon />
            New actor
          </button>
        }
      />
      <div className="split">
        <aside className="split-list" aria-label="Actors">
          <div className="split-list-search">
            <SearchIcon />
            <input
              className="input"
              value={filter}
              placeholder="Filter by name, address or label"
              aria-label="Filter actors"
              onChange={(event) => setFilter(event.target.value)}
            />
          </div>
          {actorsQuery.isLoading ? <p className="status-text split-pad">Loading actors…</p> : null}
          {actorsQuery.error ? <p className="error-text split-pad">{actorsQuery.error.message}</p> : null}
          <ul className="list">
            {visibleActors.map((actor) => (
              <li key={actor.id}>
                <button
                  type="button"
                  className={`list-row${actor.id === selectedID && mode.kind === "view" ? " is-selected" : ""}`}
                  aria-current={actor.id === selectedID && mode.kind === "view" ? "true" : undefined}
                  onClick={() => {
                    setSelectedID(actor.id);
                    setMode({ kind: "view" });
                  }}
                >
                  <span className="swatch" style={{ background: actor.color || "#4ca3ff" }} />
                  <span className="list-row-main">
                    <span className="list-row-title">{actor.name}</span>
                    <span className="list-row-meta">
                      {pluralize(actor.addresses.length, "address", "addresses")} on {pluralize(chainCounts(actor).length, "chain")}
                    </span>
                  </span>
                  <ActorHoldingsHint actorID={actor.id} />
                </button>
              </li>
            ))}
          </ul>
          {!actorsQuery.isLoading && !actors.length ? (
            <div className="empty-state split-pad">
              <strong>No actors yet</strong>
              An actor is a named set of addresses you want to follow together, like a treasury or an exchange's hot wallets.
              <button type="button" className="btn btn-sm btn-primary" onClick={() => setMode({ kind: "new" })}>
                Create the first actor
              </button>
            </div>
          ) : null}
          {actors.length && !visibleActors.length ? <p className="section-note split-pad">No actors match that filter.</p> : null}
        </aside>

        <section className="split-detail" aria-label="Actor details">
          {mode.kind === "new" || editing ? (
            <>
              <h2 className="split-detail-title">{editing ? `Edit ${editing.name}` : "New actor"}</h2>
              <ActorEditor
                key={editing ? `edit-${editing.id}` : "new"}
                initial={editing ? draftFromActor(editing) : NEW_ACTOR}
                annotations={annotationsQuery.data ?? []}
                saving={saveMutation.isPending}
                error={formError}
                onSave={(draft) => saveMutation.mutate(draft)}
                onCancel={() => {
                  setFormError("");
                  setMode({ kind: "view" });
                }}
              />
            </>
          ) : selected ? (
            <ActorDetail
              actor={selected}
              onEdit={() => {
                setFormError("");
                setMode({ kind: "edit", actorID: selected.id });
              }}
              onGraph={() => navigate("graph", { actors: String(selected.id), run: "1" })}
              onAddToCase={() => addToCase([{ kind: "actor", ref: String(selected.id) }], selected.name)}
              onDelete={() => setConfirmDelete(selected)}
              onApplyChainHints={() => void applyChainHints(selected)}
              onExploreAddress={(address) => navigate("explorer", { address: address.address })}
              onTraceAddress={(address) => navigate("trace", { seed: encodeSeed(address.address, address.chain_hint) })}
              onAddAddressToCase={(address) =>
                addToCase(
                  [{ kind: "address", ref: encodeSeed(address.address, address.chain_hint) }],
                  address.label || middleTruncate(address.address)
                )
              }
            />
          ) : !actorsQuery.isLoading && actors.length ? (
            <p className="empty-state">Pick an actor on the left to see its addresses and holdings.</p>
          ) : null}
        </section>
      </div>

      {confirmDelete ? (
        <ConfirmDialog
          title={`Delete ${confirmDelete.name}?`}
          message={`This removes the actor and its ${pluralize(confirmDelete.addresses.length, "address", "addresses")} from the directory. Graphs and traces you already saved keep their data.`}
          confirmLabel="Delete actor"
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => {
            const target = confirmDelete;
            setConfirmDelete(null);
            void deleteMutation.mutateAsync(target.id).then(() => {
              toast(`Deleted ${target.name}`);
              if (selectedID === target.id) {
                setSelectedID(null);
              }
            });
          }}
        />
      ) : null}
    </>
  );
}

function ActorHoldingsHint({ actorID }: { actorID: number }) {
  const monitorQuery = useQuery({ queryKey: ["actor-monitor", actorID], queryFn: () => getActorMonitor(actorID), staleTime: 60_000 });
  const latest = monitorQuery.data?.latest;
  if (!latest) {
    return null;
  }
  return <span className="list-row-side num">{formatCompactUSD(latest.total_usd)}</span>;
}

interface ActorDetailProps {
  actor: Actor;
  onEdit: () => void;
  onGraph: () => void;
  onAddToCase: () => void;
  onDelete: () => void;
  onApplyChainHints: () => void;
  onExploreAddress: (address: ActorAddress) => void;
  onTraceAddress: (address: ActorAddress) => void;
  onAddAddressToCase: (address: ActorAddress) => void;
}

function ActorDetail({
  actor,
  onEdit,
  onGraph,
  onAddToCase,
  onDelete,
  onApplyChainHints,
  onExploreAddress,
  onTraceAddress,
  onAddAddressToCase,
}: ActorDetailProps) {
  const counts = chainCounts(actor);
  const missing = missingChainHints(actor);
  const grouped = useMemo(() => {
    const groups = new Map<string, ActorAddress[]>();
    actor.addresses.forEach((address) => {
      const chain = (address.chain_hint || guessAddressChain(address.address)?.chain || "Unknown").toUpperCase();
      groups.set(chain, [...(groups.get(chain) ?? []), address]);
    });
    return Array.from(groups.entries()).sort((left, right) => right[1].length - left[1].length);
  }, [actor.addresses]);

  return (
    <div className="actor-detail">
      <div className="actor-detail-head">
        <div className="actor-detail-title">
          <span className="swatch swatch-lg" style={{ background: actor.color || "#4ca3ff" }} />
          <div>
            <h2>{actor.name}</h2>
            <p className="section-note">{actor.notes || "No notes"}</p>
          </div>
        </div>
        <div className="form-actions">
          <button type="button" className="btn btn-primary btn-sm" onClick={onGraph}>
            <GraphIcon />
            Graph this actor
          </button>
          <button type="button" className="btn btn-sm" onClick={onEdit}>
            <EditIcon />
            Edit
          </button>
          <button type="button" className="btn btn-sm" onClick={onAddToCase}>
            <PinIcon />
            Add to case
          </button>
          <MenuButton
            label={`More actions for ${actor.name}`}
            className="btn btn-sm btn-icon"
            items={[{ label: "Delete actor…", icon: <TrashIcon />, danger: true, onSelect: onDelete }]}
          />
        </div>
      </div>

      <div className="chip-set">
        {counts.map(([chain, count]) => (
          <span key={chain} className="chip">
            <ChainBadge chain={chain} count={count} />
          </span>
        ))}
      </div>

      {missing.length ? (
        <div className="notice" role="note">
          <span>
            {pluralize(missing.length, "address", "addresses")} {missing.length === 1 ? "has" : "have"} no chain set, so live holdings
            can't look {missing.length === 1 ? "it" : "them"} up. Detected:{" "}
            {Array.from(new Set(missing.map(({ guess }) => guess!.chain))).join(", ")}.
          </span>
          <button type="button" className="btn btn-sm" onClick={onApplyChainHints}>
            Set the detected chains
          </button>
        </div>
      ) : null}

      <section className="section" aria-label="Addresses">
        <h3>
          Addresses <span className="count">{actor.addresses.length}</span>
        </h3>
        {grouped.map(([chain, addresses]) => (
          <div key={chain} className="address-group">
            <div className="address-group-head">
              <ChainBadge chain={chain} count={addresses.length} />
            </div>
            <ul className="list ws-run-list">
              {addresses.map((address) => (
                <li key={address.id} className="list-row">
                  <span className="list-row-main">
                    <AddressText value={address.address} head={12} tail={8} />
                    {address.label ? <span className="list-row-meta">{address.label}</span> : null}
                  </span>
                  <button
                    type="button"
                    className="btn btn-ghost btn-icon btn-sm"
                    title="Explore this address"
                    aria-label={`Explore ${address.address}`}
                    onClick={() => onExploreAddress(address)}
                  >
                    <ExplorerIcon />
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-icon btn-sm"
                    title="Trace funds from this address"
                    aria-label={`Trace from ${address.address}`}
                    onClick={() => onTraceAddress(address)}
                  >
                    <TraceIcon />
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-icon btn-sm"
                    title="Add to the active case"
                    aria-label={`Add ${address.address} to the active case`}
                    onClick={() => onAddAddressToCase(address)}
                  >
                    <PinIcon />
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </section>

      <ActorMonitorPanel key={actor.id} actor={actor} />
    </div>
  );
}
