import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addToBlocklist,
  deleteAnnotation,
  listAnnotations,
  listBlocklist,
  removeFromBlocklist,
  upsertAnnotation,
} from "../../lib/api";
import { formatDateTime } from "../../lib/format";

export function AnnotationsPage() {
  const queryClient = useQueryClient();
  const annotationsQuery = useQuery({
    queryKey: ["annotations"],
    queryFn: listAnnotations,
  });
  const blocklistQuery = useQuery({
    queryKey: ["blocklist"],
    queryFn: listBlocklist,
  });
  const [annotationDraft, setAnnotationDraft] = useState({
    address: "",
    kind: "label",
    value: "",
  });
  const [blocklistDraft, setBlocklistDraft] = useState({
    address: "",
    reason: "",
  });

  const annotationMutation = useMutation({
    mutationFn: upsertAnnotation,
    onSuccess: async () => {
      setAnnotationDraft({ address: "", kind: "label", value: "" });
      await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    },
  });

  const annotationDeleteMutation = useMutation({
    mutationFn: deleteAnnotation,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    },
  });

  const blocklistMutation = useMutation({
    mutationFn: addToBlocklist,
    onSuccess: async () => {
      setBlocklistDraft({ address: "", reason: "" });
      await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
    },
  });

  const blocklistDeleteMutation = useMutation({
    mutationFn: removeFromBlocklist,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
    },
  });

  return (
    <div className="page-grid two-up">
      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Annotations</span>
            <h2>Address Metadata</h2>
          </div>
          <span className="status-pill ok">{annotationsQuery.data?.length ?? 0}</span>
        </div>
        <form
          className="form-grid"
          onSubmit={(event) => {
            event.preventDefault();
            annotationMutation.mutate(annotationDraft);
          }}
        >
          <label className="field field-full">
            <span>Address</span>
            <input
              value={annotationDraft.address}
              onChange={(event) => setAnnotationDraft((current) => ({ ...current, address: event.target.value }))}
              placeholder="thor1..."
            />
          </label>
          <label className="field">
            <span>Kind</span>
            <input
              value={annotationDraft.kind}
              onChange={(event) => setAnnotationDraft((current) => ({ ...current, kind: event.target.value }))}
              placeholder="label"
            />
          </label>
          <label className="field">
            <span>Value</span>
            <input
              value={annotationDraft.value}
              onChange={(event) => setAnnotationDraft((current) => ({ ...current, value: event.target.value }))}
              placeholder="Treasury hot wallet"
            />
          </label>
          <div className="form-actions field-full">
            <button type="submit" className="button" disabled={annotationMutation.isPending}>
              {annotationMutation.isPending ? "Saving..." : "Save Annotation"}
            </button>
          </div>
        </form>
        {annotationsQuery.isLoading ? <div className="empty-state">Loading annotations…</div> : null}
        {annotationsQuery.error ? <p className="error-text">{annotationsQuery.error.message}</p> : null}
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Address</th>
                <th>Kind</th>
                <th>Value</th>
                <th>Created</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(annotationsQuery.data ?? []).map((annotation) => (
                <tr key={`${annotation.normalized_address}:${annotation.kind}`}>
                  <td className="mono-wrap">{annotation.address}</td>
                  <td>{annotation.kind}</td>
                  <td>{annotation.value}</td>
                  <td>{formatDateTime(annotation.created_at)}</td>
                  <td>
                    <button
                      type="button"
                      className="button secondary slim danger"
                      onClick={() =>
                        annotationDeleteMutation.mutate({
                          address: annotation.address,
                          kind: annotation.kind,
                        })
                      }
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="panel page-panel">
        <div className="panel-head">
          <div>
            <span className="eyebrow">Blocklist</span>
            <h2>Excluded Addresses</h2>
          </div>
          <span className="status-pill ok">{blocklistQuery.data?.length ?? 0}</span>
        </div>
        <form
          className="form-grid"
          onSubmit={(event) => {
            event.preventDefault();
            blocklistMutation.mutate(blocklistDraft);
          }}
        >
          <label className="field field-full">
            <span>Address</span>
            <input
              value={blocklistDraft.address}
              onChange={(event) => setBlocklistDraft((current) => ({ ...current, address: event.target.value }))}
              placeholder="0x..."
            />
          </label>
          <label className="field field-full">
            <span>Reason</span>
            <input
              value={blocklistDraft.reason}
              onChange={(event) => setBlocklistDraft((current) => ({ ...current, reason: event.target.value }))}
              placeholder="Removed from graph"
            />
          </label>
          <div className="form-actions field-full">
            <button type="submit" className="button" disabled={blocklistMutation.isPending}>
              {blocklistMutation.isPending ? "Saving..." : "Add to Blocklist"}
            </button>
          </div>
        </form>
        {blocklistQuery.isLoading ? <div className="empty-state">Loading blocklist…</div> : null}
        {blocklistQuery.error ? <p className="error-text">{blocklistQuery.error.message}</p> : null}
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Address</th>
                <th>Reason</th>
                <th>Created</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(blocklistQuery.data ?? []).map((entry) => (
                <tr key={entry.normalized_address}>
                  <td className="mono-wrap">{entry.address}</td>
                  <td>{entry.reason || "n/a"}</td>
                  <td>{formatDateTime(entry.created_at)}</td>
                  <td>
                    <button
                      type="button"
                      className="button secondary slim danger"
                      onClick={() => blocklistDeleteMutation.mutate(entry.address)}
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
