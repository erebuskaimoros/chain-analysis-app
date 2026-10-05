import { useMemo, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PlusIcon, SearchIcon } from "../../app/icons";
import { useRouteIntent, useRouter } from "../../app/router";
import { useToast } from "../../app/toast";
import {
  addToBlocklist,
  deleteAnnotation,
  listAnnotations,
  listBlocklist,
  removeFromBlocklist,
  upsertAnnotation,
} from "../../lib/api";
import { formatDateTime, formatRelativeTime, middleTruncate, pluralize } from "../../lib/format";
import { encodeSeed, guessAddressChain } from "../../lib/identify";
import type { AddressAnnotation, BlocklistedAddress } from "../../lib/types";
import { AddressText } from "../../ui/AddressText";
import { ConfirmDialog } from "../../ui/Dialog";
import { MenuButton } from "../../ui/Menu";
import { PageHeader } from "../../ui/PageHeader";

type Tab = "labels" | "excluded";

const KIND_OPTIONS = [
  { value: "label", label: "Label" },
  { value: "asgard_vault", label: "Asgard vault" },
];

function kindName(kind: string) {
  return KIND_OPTIONS.find((option) => option.value === kind)?.label ?? kind.replace(/_/g, " ");
}

interface AnnotationDraft {
  address: string;
  kind: string;
  value: string;
}

const EMPTY_DRAFT: AnnotationDraft = { address: "", kind: "label", value: "" };

export function AnnotationsPage() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { navigate } = useRouter();
  const annotationsQuery = useQuery({ queryKey: ["annotations"], queryFn: listAnnotations });
  const blocklistQuery = useQuery({ queryKey: ["blocklist"], queryFn: listBlocklist });
  const [tab, setTab] = useState<Tab>("labels");
  const [filter, setFilter] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [draft, setDraft] = useState<AnnotationDraft>(EMPTY_DRAFT);
  const [editing, setEditing] = useState<{ address: string; kind: string } | null>(null);
  const [customKind, setCustomKind] = useState(false);
  const [excludeDraft, setExcludeDraft] = useState({ address: "", reason: "" });
  const [excludeOpen, setExcludeOpen] = useState(false);
  const [confirm, setConfirm] = useState<{ kind: "annotation"; item: AddressAnnotation } | { kind: "excluded"; item: BlocklistedAddress } | null>(
    null
  );

  useRouteIntent("annotations", (params) => {
    if (params.address) {
      setTab("labels");
      setEditing(null);
      setCustomKind(false);
      setDraft({ address: params.address, kind: "label", value: "" });
      setFormOpen(true);
    }
    if (params.tab === "excluded") {
      setTab("excluded");
    }
  });

  const annotationMutation = useMutation({
    mutationFn: upsertAnnotation,
    onSuccess: async (_data, variables) => {
      closeForm();
      await queryClient.invalidateQueries({ queryKey: ["annotations"] });
      toast(`Saved ${kindName(variables.kind).toLowerCase()} for ${middleTruncate(variables.address)}`);
    },
  });

  const annotationDeleteMutation = useMutation({
    mutationFn: deleteAnnotation,
    onSuccess: async (_data, variables) => {
      if (editing && editing.address === variables.address && editing.kind === variables.kind) {
        closeForm();
      }
      await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    },
  });

  const blocklistMutation = useMutation({
    mutationFn: addToBlocklist,
    onSuccess: async (_data, variables) => {
      setExcludeDraft({ address: "", reason: "" });
      setExcludeOpen(false);
      await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
      toast(`Excluded ${middleTruncate(variables.address)} from all graphs`);
    },
  });

  const blocklistDeleteMutation = useMutation({
    mutationFn: removeFromBlocklist,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
    },
  });

  function startEditing(annotation: AddressAnnotation) {
    setEditing({ address: annotation.address, kind: annotation.kind });
    setDraft({ address: annotation.address, kind: annotation.kind, value: annotation.value });
    setCustomKind(!KIND_OPTIONS.some((option) => option.value === annotation.kind));
    setFormOpen(true);
  }

  function closeForm() {
    setEditing(null);
    setDraft(EMPTY_DRAFT);
    setCustomKind(false);
    setFormOpen(false);
  }

  const needle = filter.trim().toLowerCase();
  const annotations = useMemo(
    () =>
      (annotationsQuery.data ?? []).filter(
        (annotation) =>
          !needle ||
          annotation.address.toLowerCase().includes(needle) ||
          annotation.value.toLowerCase().includes(needle) ||
          annotation.kind.toLowerCase().includes(needle)
      ),
    [annotationsQuery.data, needle]
  );
  const excluded = useMemo(
    () =>
      (blocklistQuery.data ?? []).filter(
        (entry) => !needle || entry.address.toLowerCase().includes(needle) || (entry.reason ?? "").toLowerCase().includes(needle)
      ),
    [blocklistQuery.data, needle]
  );
  const isEditing = editing !== null;
  const totalLabels = annotationsQuery.data?.length ?? 0;
  const totalExcluded = blocklistQuery.data?.length ?? 0;

  function submitAnnotation(event: FormEvent) {
    event.preventDefault();
    annotationMutation.mutate({ address: draft.address.trim(), kind: draft.kind.trim(), value: draft.value.trim() });
  }

  function addressActions(address: string) {
    const guess = guessAddressChain(address);
    return [
      { label: "Explore this address", onSelect: () => navigate("explorer", { address }) },
      {
        label: "Trace funds from here",
        onSelect: () => navigate("trace", { seed: guess?.certain ? encodeSeed(address, guess.chain) : address }),
      },
      { label: "Copy address", onSelect: () => void navigator.clipboard?.writeText(address) },
    ];
  }

  return (
    <>
      <PageHeader
        title="Labels"
        context={`${pluralize(totalLabels, "label")}, ${pluralize(totalExcluded, "excluded address", "excluded addresses")}`}
        actions={
          tab === "labels" ? (
            <button
              type="button"
              className="btn btn-sm btn-primary"
              onClick={() => {
                setEditing(null);
                setDraft(EMPTY_DRAFT);
                setCustomKind(false);
                setFormOpen(true);
              }}
            >
              <PlusIcon />
              New label
            </button>
          ) : (
            <button type="button" className="btn btn-sm btn-primary" onClick={() => setExcludeOpen(true)}>
              <PlusIcon />
              Exclude an address
            </button>
          )
        }
      />
      <div className="page-body labels-page">
        <div className="labels-toolbar">
          <div className="inspector-tabs labels-tabs" role="tablist" aria-label="Labels and exclusions">
            <button type="button" role="tab" aria-selected={tab === "labels"} onClick={() => setTab("labels")}>
              Address labels <span className="count">{totalLabels}</span>
            </button>
            <button type="button" role="tab" aria-selected={tab === "excluded"} onClick={() => setTab("excluded")}>
              Excluded from graphs <span className="count">{totalExcluded}</span>
            </button>
          </div>
          <div className="split-list-search labels-search">
            <SearchIcon />
            <input
              className="input"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              placeholder="Filter by address, label or kind"
              aria-label="Filter labels"
            />
          </div>
        </div>

        {tab === "labels" ? (
          <section className="section" aria-label="Address labels">
            <p className="section-note">
              Labels name addresses on graphs, in search and in reports. An Asgard vault mark tells the graph to treat the
              address as a THORChain vault.
            </p>
            {formOpen ? (
              <form className="labels-form" onSubmit={submitAnnotation} aria-label={isEditing ? "Edit label" : "New label"}>
                {isEditing ? (
                  <p className="section-note">
                    Editing the {kindName(draft.kind).toLowerCase()} for <span className="mono">{draft.address}</span>. The address
                    and kind stay fixed; change the value.
                  </p>
                ) : null}
                <div className="labels-form-grid">
                  <label className="field">
                    <span>Address</span>
                    <input
                      className="mono"
                      value={draft.address}
                      onChange={(event) => setDraft((current) => ({ ...current, address: event.target.value }))}
                      placeholder="thor1..."
                      disabled={isEditing}
                      autoFocus={!isEditing && !draft.address}
                      spellCheck={false}
                    />
                  </label>
                  <div className="field">
                    <span>Kind</span>
                    {customKind || isEditing ? (
                      <input
                        value={draft.kind}
                        onChange={(event) => setDraft((current) => ({ ...current, kind: event.target.value }))}
                        placeholder="label"
                        disabled={isEditing}
                        aria-label="Kind"
                      />
                    ) : (
                      <select
                        aria-label="Kind"
                        value={draft.kind}
                        onChange={(event) => {
                          if (event.target.value === "__custom") {
                            setCustomKind(true);
                            setDraft((current) => ({ ...current, kind: "" }));
                            return;
                          }
                          setDraft((current) => ({ ...current, kind: event.target.value }));
                        }}
                      >
                        {KIND_OPTIONS.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                        <option value="__custom">Other kind…</option>
                      </select>
                    )}
                  </div>
                  <label className="field">
                    <span>Value</span>
                    <input
                      value={draft.value}
                      autoFocus={isEditing || Boolean(draft.address)}
                      onChange={(event) => setDraft((current) => ({ ...current, value: event.target.value }))}
                      placeholder={draft.kind === "asgard_vault" ? "true" : "Treasury hot wallet"}
                    />
                  </label>
                </div>
                <div className="form-actions">
                  <button
                    type="submit"
                    className="btn btn-primary btn-sm"
                    disabled={annotationMutation.isPending || !draft.address.trim() || !draft.kind.trim()}
                  >
                    {annotationMutation.isPending ? "Saving…" : isEditing ? "Save changes" : "Save label"}
                  </button>
                  <button type="button" className="btn btn-sm" onClick={closeForm}>
                    Cancel
                  </button>
                </div>
                {annotationMutation.error ? <p className="form-error">{annotationMutation.error.message}</p> : null}
              </form>
            ) : null}

            {annotationsQuery.isLoading ? <p className="status-text">Loading labels…</p> : null}
            {annotationsQuery.error ? <p className="error-text">{annotationsQuery.error.message}</p> : null}
            {annotations.length ? (
              <div className="table-wrap trace-table-wrap labels-table">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Address</th>
                      <th>Kind</th>
                      <th>Value</th>
                      <th>Added</th>
                      <th className="cell-actions" aria-label="Row actions" />
                    </tr>
                  </thead>
                  <tbody>
                    {annotations.map((annotation) => (
                      <tr key={`${annotation.normalized_address}:${annotation.kind}`}>
                        <td>
                          <AddressText value={annotation.address} head={10} tail={8} />
                        </td>
                        <td>
                          <span className="chip">{kindName(annotation.kind)}</span>
                        </td>
                        <td>{annotation.value}</td>
                        <td className="cell-muted" title={formatDateTime(annotation.created_at)}>
                          {formatRelativeTime(annotation.created_at)}
                        </td>
                        <td className="cell-actions">
                          <MenuButton
                            label={`Actions for ${annotation.value || annotation.address}`}
                            items={[
                              { label: "Edit", onSelect: () => startEditing(annotation) },
                              ...addressActions(annotation.address),
                              "separator",
                              { label: "Delete…", danger: true, onSelect: () => setConfirm({ kind: "annotation", item: annotation }) },
                            ]}
                          />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : !annotationsQuery.isLoading ? (
              <p className="empty-state">
                {needle ? "No labels match that filter." : "No labels yet. Label addresses from the graph, from search, or with New label."}
              </p>
            ) : null}
          </section>
        ) : (
          <section className="section" aria-label="Excluded addresses">
            <p className="section-note">
              Excluded addresses are left out of every graph, for example noisy routers or your own test wallets.
            </p>
            {excludeOpen ? (
              <form
                className="labels-form"
                onSubmit={(event) => {
                  event.preventDefault();
                  blocklistMutation.mutate({ address: excludeDraft.address.trim(), reason: excludeDraft.reason.trim() });
                }}
              >
                <div className="labels-form-grid">
                  <label className="field">
                    <span>Address</span>
                    <input
                      className="mono"
                      value={excludeDraft.address}
                      autoFocus
                      onChange={(event) => setExcludeDraft((current) => ({ ...current, address: event.target.value }))}
                      placeholder="0x..."
                      spellCheck={false}
                    />
                  </label>
                  <label className="field labels-form-wide">
                    <span>Reason</span>
                    <input
                      value={excludeDraft.reason}
                      onChange={(event) => setExcludeDraft((current) => ({ ...current, reason: event.target.value }))}
                      placeholder="Router noise"
                    />
                  </label>
                </div>
                <div className="form-actions">
                  <button
                    type="submit"
                    className="btn btn-primary btn-sm"
                    disabled={blocklistMutation.isPending || !excludeDraft.address.trim()}
                  >
                    {blocklistMutation.isPending ? "Saving…" : "Exclude address"}
                  </button>
                  <button type="button" className="btn btn-sm" onClick={() => setExcludeOpen(false)}>
                    Cancel
                  </button>
                </div>
              </form>
            ) : null}
            {blocklistQuery.isLoading ? <p className="status-text">Loading…</p> : null}
            {blocklistQuery.error ? <p className="error-text">{blocklistQuery.error.message}</p> : null}
            {excluded.length ? (
              <div className="table-wrap trace-table-wrap labels-table">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Address</th>
                      <th>Reason</th>
                      <th>Added</th>
                      <th className="cell-actions" aria-label="Row actions" />
                    </tr>
                  </thead>
                  <tbody>
                    {excluded.map((entry) => (
                      <tr key={entry.normalized_address}>
                        <td>
                          <AddressText value={entry.address} head={10} tail={8} />
                        </td>
                        <td>{entry.reason || "No reason given"}</td>
                        <td className="cell-muted" title={formatDateTime(entry.created_at)}>
                          {formatRelativeTime(entry.created_at)}
                        </td>
                        <td className="cell-actions">
                          <MenuButton
                            label={`Actions for ${entry.address}`}
                            items={[
                              ...addressActions(entry.address),
                              "separator",
                              { label: "Include in graphs again", onSelect: () => setConfirm({ kind: "excluded", item: entry }) },
                            ]}
                          />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : !blocklistQuery.isLoading ? (
              <p className="empty-state">{needle ? "No excluded addresses match that filter." : "No addresses are excluded."}</p>
            ) : null}
          </section>
        )}
      </div>

      {confirm?.kind === "annotation" ? (
        <ConfirmDialog
          title="Delete this label?"
          message={
            <>
              The {kindName(confirm.item.kind).toLowerCase()} <strong>{confirm.item.value}</strong> on{" "}
              <span className="mono">{middleTruncate(confirm.item.address)}</span> will be removed from graphs and search.
            </>
          }
          confirmLabel="Delete label"
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            const item = confirm.item;
            setConfirm(null);
            annotationDeleteMutation.mutate({ address: item.address, kind: item.kind });
          }}
        />
      ) : null}
      {confirm?.kind === "excluded" ? (
        <ConfirmDialog
          title="Include this address in graphs again?"
          message={
            <>
              <span className="mono">{middleTruncate(confirm.item.address)}</span> will show up in graphs you build from now on.
            </>
          }
          confirmLabel="Include again"
          danger={false}
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            const item = confirm.item;
            setConfirm(null);
            blocklistDeleteMutation.mutate(item.address);
          }}
        />
      ) : null}
    </>
  );
}
