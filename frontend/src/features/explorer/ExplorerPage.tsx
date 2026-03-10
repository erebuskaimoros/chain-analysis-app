import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  buildAddressExplorer,
  deleteAddressExplorerRun,
  listAddressExplorerRuns,
  lookupAction,
} from "../../lib/api";
import { DEFAULT_FLOW_TYPES } from "../../lib/constants";
import { formatShortDateTime, formatUSD } from "../../lib/format";
import { mergeAddressExplorerResponse, type GraphSelection } from "../../lib/graph";
import type { ActionLookupResponse, AddressExplorerRequest, AddressExplorerResponse } from "../../lib/types";
import { GraphCanvas } from "../shared/GraphCanvas";
import { SelectionInspector } from "../shared/SelectionInspector";
import { ActionLookupPanel } from "../shared/ActionLookupPanel";
import { SupportingActionsTable } from "../shared/SupportingActionsTable";

interface ExplorerFormState {
  address: string;
  min_usd: string;
  batch_size: number;
}

function explorerRequest(
  form: ExplorerFormState,
  mode: "preview" | "graph",
  direction: "" | "newest" | "oldest",
  offset: number
): AddressExplorerRequest {
  const minUSD = Number(form.min_usd);
  return {
    address: form.address.trim(),
    flow_types: [...DEFAULT_FLOW_TYPES],
    min_usd: Number.isFinite(minUSD) ? minUSD : 0,
    mode,
    direction,
    offset,
    batch_size: Number(form.batch_size) || 10,
  };
}

export function ExplorerPage() {
  const queryClient = useQueryClient();
  const runsQuery = useQuery({
    queryKey: ["address-explorer-runs"],
    queryFn: listAddressExplorerRuns,
  });

  const [form, setForm] = useState<ExplorerFormState>({
    address: "",
    min_usd: "0",
    batch_size: 10,
  });
  const [preview, setPreview] = useState<AddressExplorerResponse | null>(null);
  const [graph, setGraph] = useState<AddressExplorerResponse | null>(null);
  const [selection, setSelection] = useState<GraphSelection>(null);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [statusText, setStatusText] = useState("Enter an address to preview address activity.");
  const [lookupResult, setLookupResult] = useState<ActionLookupResponse | null>(null);
  const [lookupError, setLookupError] = useState("");

  const previewMutation = useMutation({
    mutationFn: buildAddressExplorer,
    onSuccess: (response) => {
      setPreview(response);
      setGraph(null);
      setSelection(null);
      setLookupResult(null);
      setLookupError("");
      setStatusText(
        response.direction_required
          ? "Choose newest or oldest to load the first graph batch."
          : "Preview complete. Loading graph..."
      );
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Explorer preview failed.");
    },
  });

  const graphMutation = useMutation({
    mutationFn: buildAddressExplorer,
    onSuccess: async (response, request) => {
      setPreview(response);
      setGraph((current) => (request.offset > 0 ? mergeAddressExplorerResponse(current, response) : response));
      setSelection(null);
      setStatusText(
        request.offset > 0
          ? `Loaded additional actions. ${response.has_more ? "More actions remain." : "No more actions remain."}`
          : `Loaded ${response.nodes.length} nodes and ${response.edges.length} edges for ${response.address}.`
      );
      if (request.offset === 0) {
        await queryClient.invalidateQueries({ queryKey: ["address-explorer-runs"] });
      }
    },
    onError: (error) => {
      setStatusText(error instanceof Error ? error.message : "Explorer graph load failed.");
    },
  });

  const deleteRunMutation = useMutation({
    mutationFn: deleteAddressExplorerRun,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["address-explorer-runs"] });
    },
  });

  const lookupMutation = useMutation({
    mutationFn: lookupAction,
    onSuccess: (response) => {
      setLookupResult(response);
      setLookupError("");
    },
    onError: (error) => {
      setLookupError(error instanceof Error ? error.message : "Action lookup failed.");
    },
  });

  const selectedRun = useMemo(
    () => runsQuery.data?.find((run) => String(run.id) === selectedRunID) ?? null,
    [runsQuery.data, selectedRunID]
  );

  async function loadGraph(direction: "newest" | "oldest", offset = 0) {
    const request = explorerRequest(form, "graph", direction, offset);
    setStatusText(offset > 0 ? "Loading more actions..." : "Loading explorer graph...");
    await graphMutation.mutateAsync(request);
  }

  async function requestPreview() {
    const request = explorerRequest(form, "preview", "", 0);
    if (!request.address) {
      setStatusText("Address is required.");
      return;
    }

    const response = await previewMutation.mutateAsync(request);
    if (!response.direction_required) {
      await loadGraph("newest", 0);
    }
  }

  async function handleLoadRun() {
    if (!selectedRun) {
      return;
    }

    setForm({
      address: selectedRun.request.address,
      min_usd: String(selectedRun.request.min_usd ?? 0),
      batch_size: selectedRun.request.batch_size || 10,
    });
    await graphMutation.mutateAsync({
      ...selectedRun.request,
      mode: "graph",
      offset: 0,
      direction: selectedRun.request.direction || "newest",
    });
  }

  async function handleDeleteRun() {
    if (!selectedRun) {
      return;
    }
    await deleteRunMutation.mutateAsync(selectedRun.id);
  }

  const currentGraph = graph;

  return (
    <div className="page-grid graph-layout">
      <div className="page-stack">
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Address Explorer</span>
              <h2>Preview and Load</h2>
            </div>
          </div>
          <form
            className="form-grid"
            onSubmit={(event) => {
              event.preventDefault();
              void requestPreview();
            }}
          >
            <label className="field field-full">
              <span>Address</span>
              <input
                value={form.address}
                onChange={(event) => setForm((current) => ({ ...current, address: event.target.value }))}
                placeholder="thor1..."
              />
            </label>
            <label className="field">
              <span>Min USD</span>
              <input
                type="number"
                step="any"
                value={form.min_usd}
                onChange={(event) => setForm((current) => ({ ...current, min_usd: event.target.value }))}
              />
            </label>
            <label className="field">
              <span>Batch Size</span>
              <input
                type="number"
                min={1}
                max={20}
                value={form.batch_size}
                onChange={(event) =>
                  setForm((current) => ({ ...current, batch_size: Number(event.target.value) || current.batch_size }))
                }
              />
            </label>
            <div className="form-actions field-full">
              <button type="submit" className="button" disabled={previewMutation.isPending || graphMutation.isPending}>
                {previewMutation.isPending ? "Checking..." : "Preview Address"}
              </button>
            </div>
          </form>
          <p className="status-line">{statusText}</p>
          {preview?.direction_required ? (
            <div className="button-row">
              <button
                type="button"
                className="button secondary"
                disabled={graphMutation.isPending}
                onClick={() => {
                  void loadGraph("newest", 0);
                }}
              >
                Load Newest
              </button>
              <button
                type="button"
                className="button secondary"
                disabled={graphMutation.isPending}
                onClick={() => {
                  void loadGraph("oldest", 0);
                }}
              >
                Load Oldest
              </button>
            </div>
          ) : null}
          {preview ? (
            <div className="chip-list">
              <span className="meta-chip">{preview.active_chains.length} active chains</span>
              <span className="meta-chip">{preview.total_estimate >= 0 ? `${preview.total_estimate} est. actions` : "Total unknown"}</span>
              <span className="meta-chip">{formatUSD(preview.query.min_usd)} min</span>
            </div>
          ) : null}
        </section>

        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Runs</span>
              <h2>Saved Explorer Runs</h2>
            </div>
            <span className="status-pill ok">{runsQuery.data?.length ?? 0}</span>
          </div>
          {runsQuery.isLoading ? <div className="empty-state">Loading runs…</div> : null}
          <div className="stack">
            <select value={selectedRunID} onChange={(event) => setSelectedRunID(event.target.value)}>
              <option value="">Select a saved run</option>
              {(runsQuery.data ?? []).map((run) => (
                <option key={run.id} value={run.id}>
                  {run.summary} · {run.node_count}N/{run.edge_count}E · {formatShortDateTime(run.created_at)}
                </option>
              ))}
            </select>
            <div className="button-row">
              <button type="button" className="button secondary" disabled={!selectedRun} onClick={() => void handleLoadRun()}>
                Load
              </button>
              <button
                type="button"
                className="button secondary danger"
                disabled={!selectedRun || deleteRunMutation.isPending}
                onClick={() => void handleDeleteRun()}
              >
                Delete
              </button>
            </div>
          </div>
        </section>
      </div>

      <div className="page-stack">
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Graph</span>
              <h2>{currentGraph ? "Explorer Graph" : "No Graph Loaded"}</h2>
            </div>
            {currentGraph ? <span className="status-pill ok">{currentGraph.loaded_actions} actions</span> : null}
          </div>
          {currentGraph ? (
            <>
              <div className="chip-list">
                <span className="meta-chip">{currentGraph.nodes.length} nodes</span>
                <span className="meta-chip">{currentGraph.edges.length} edges</span>
                <span className="meta-chip">{currentGraph.query.direction || "newest"} direction</span>
              </div>
              {currentGraph.warnings.length ? (
                <div className="warning-list">
                  {currentGraph.warnings.map((warning) => (
                    <span key={warning} className="warning-chip">
                      {warning}
                    </span>
                  ))}
                </div>
              ) : null}
              <GraphCanvas
                nodes={currentGraph.nodes}
                edges={currentGraph.edges}
                selection={selection}
                onSelectionChange={setSelection}
              />
              {currentGraph.has_more ? (
                <div className="button-row">
                  <button
                    type="button"
                    className="button secondary"
                    disabled={graphMutation.isPending}
                    onClick={() => {
                      void loadGraph((currentGraph.query.direction || "newest") as "newest" | "oldest", currentGraph.next_offset);
                    }}
                  >
                    {graphMutation.isPending ? "Loading..." : "Load Next Batch"}
                  </button>
                </div>
              ) : null}
            </>
          ) : (
            <div className="empty-state">Preview an address and load a graph batch to inspect one-hop activity.</div>
          )}
        </section>

        <div className="page-grid inspector-grid">
          <section className="panel page-panel">
            <div className="panel-head">
              <div>
                <span className="eyebrow">Actions</span>
                <h2>Supporting Actions</h2>
              </div>
            </div>
            <SupportingActionsTable
              actions={currentGraph?.supporting_actions ?? []}
              onLookup={(txID) => lookupMutation.mutate(txID)}
            />
          </section>

          <section className="panel page-panel">
            <div className="panel-head">
              <div>
                <span className="eyebrow">Inspector</span>
                <h2>Selection Detail</h2>
              </div>
            </div>
            <SelectionInspector
              selection={selection}
              emptyMessage="Select a graph node or edge to inspect explorer graph metadata."
              onLookupTx={(txID) => lookupMutation.mutate(txID)}
            />
          </section>

          <section className="panel page-panel">
            <div className="panel-head">
              <div>
                <span className="eyebrow">Lookup</span>
                <h2>Action Detail</h2>
              </div>
            </div>
            <ActionLookupPanel
              result={lookupResult}
              isLoading={lookupMutation.isPending}
              error={lookupError}
            />
          </section>
        </div>
      </div>
    </div>
  );
}
