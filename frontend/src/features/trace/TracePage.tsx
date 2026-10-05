import { useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteTraceRun, getTraceRun, listTraceRuns, startTrace } from "../../lib/api";
import { formatShortDateTime, formatUSD } from "../../lib/format";
import type { TraceDirection, TracePolicy, TraceRequest, TraceResponse, TraceSeed } from "../../lib/types";
import { TraceResults } from "./TraceResults";

const STOP_CATEGORIES = ["exchange", "sanctioned", "mixer", "scam", "hack", "bridge", "defi"];
const DEFAULT_STOPS = ["exchange", "sanctioned", "mixer"];

export interface TraceFormState {
  seeds: string;
  startTime: string;
  endTime: string;
  direction: TraceDirection;
  policy: TracePolicy;
  amount: string;
  asset: string;
  maxDepth: string;
  maxBranches: string;
  minUSD: string;
  stopCategories: string[];
  includeHoldings: boolean;
}

function utcInputValue(date: Date) {
  return date.toISOString().slice(0, 16);
}

function defaultForm(): TraceFormState {
  const end = new Date();
  const start = new Date(end.getTime() - 7 * 24 * 3600 * 1000);
  return {
    seeds: "",
    startTime: utcInputValue(start),
    endTime: utcInputValue(end),
    direction: "forward",
    policy: "fifo",
    amount: "",
    asset: "",
    maxDepth: "3",
    maxBranches: "8",
    minUSD: "0",
    stopCategories: DEFAULT_STOPS,
    includeHoldings: true,
  };
}

// parseTraceSeeds reads one seed per line: a 64-hex transaction hash, an
// address, or CHAIN|address.
export function parseTraceSeeds(text: string): TraceSeed[] {
  return text
    .split(/\n|,/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      if (/^(0x)?[0-9a-fA-F]{64}$/.test(line)) {
        return { tx_id: line };
      }
      const [chain, address] = line.includes("|") ? line.split("|", 2) : ["", line];
      return chain ? { chain: chain.trim().toUpperCase(), address: address.trim() } : { address: line };
    });
}

export function buildTraceRequest(form: TraceFormState): TraceRequest {
  const amount = Number(form.amount);
  return {
    seeds: parseTraceSeeds(form.seeds),
    start_time: form.startTime ? `${form.startTime}:00Z` : "",
    end_time: form.endTime ? `${form.endTime}:00Z` : undefined,
    direction: form.direction,
    policy: form.policy,
    amount: amount > 0 ? amount : undefined,
    asset: amount > 0 ? form.asset.trim().toUpperCase() : undefined,
    max_depth: Number(form.maxDepth) || undefined,
    max_branches: Number(form.maxBranches) || undefined,
    min_usd_at_time: Number(form.minUSD) || 0,
    stop_categories: form.stopCategories,
    include_holdings: form.includeHoldings,
  };
}

function formFromRequest(request: TraceRequest): TraceFormState {
  const seeds = request.seeds
    .map((seed) => seed.tx_id || (seed.chain ? `${seed.chain}|${seed.address}` : seed.address) || "")
    .join("\n");
  return {
    seeds,
    startTime: request.start_time ? utcInputValue(new Date(request.start_time)) : "",
    endTime: request.end_time ? utcInputValue(new Date(request.end_time)) : "",
    direction: request.direction ?? "forward",
    policy: request.policy ?? "fifo",
    amount: request.amount ? String(request.amount) : "",
    asset: request.asset ?? "",
    maxDepth: String(request.max_depth ?? 3),
    maxBranches: String(request.max_branches ?? 8),
    minUSD: String(request.min_usd_at_time ?? 0),
    stopCategories: request.stop_categories ?? DEFAULT_STOPS,
    includeHoldings: Boolean(request.include_holdings),
  };
}

export function TracePage() {
  const queryClient = useQueryClient();
  const runsQuery = useQuery({ queryKey: ["trace-runs"], queryFn: listTraceRuns });
  const [form, setForm] = useState<TraceFormState>(defaultForm);
  const [result, setResult] = useState<TraceResponse | null>(null);
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);

  function update<K extends keyof TraceFormState>(key: K, value: TraceFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    const request = buildTraceRequest(form);
    if (!request.seeds.length) {
      setStatus("Enter at least one address or transaction hash.");
      return;
    }
    setBusy(true);
    setStatus("Tracing…");
    try {
      const response = await startTrace(request, (job) =>
        setStatus(`Tracing — ${job.stage || "starting"}${job.message ? `: ${job.message}` : ""}…`)
      );
      setResult(response);
      setStatus(`Traced ${response.edges.length} edges to ${response.sinks.length} sinks.`);
      await queryClient.invalidateQueries({ queryKey: ["trace-runs"] });
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Trace failed.");
    } finally {
      setBusy(false);
    }
  }

  async function onOpenRun(id: number) {
    setStatus("Loading saved trace…");
    try {
      const run = await getTraceRun(id);
      setForm(formFromRequest(run.request));
      setResult(run.response ?? null);
      setStatus(`Loaded "${run.title}".`);
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Could not load the trace.");
    }
  }

  async function onDeleteRun(id: number) {
    await deleteTraceRun(id);
    await queryClient.invalidateQueries({ queryKey: ["trace-runs"] });
  }

  const amountSet = Number(form.amount) > 0;

  return (
    <div className="page-stack">
      <div className="page-grid two-up">
        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">Follow the funds</span>
              <h2>Trace</h2>
            </div>
          </div>
          <form className="form-grid" onSubmit={(event) => void onSubmit(event)}>
            <label className="field field-full">
              <span>Seeds (one address, CHAIN|address, or transaction hash per line)</span>
              <textarea rows={3} value={form.seeds} onChange={(event) => update("seeds", event.target.value)} placeholder="ETH|0x…" />
            </label>
            <label className="field">
              <span>Start (UTC)</span>
              <input type="datetime-local" value={form.startTime} onChange={(event) => update("startTime", event.target.value)} />
            </label>
            <label className="field">
              <span>End (UTC)</span>
              <input type="datetime-local" value={form.endTime} onChange={(event) => update("endTime", event.target.value)} />
            </label>
            <label className="field">
              <span>Direction</span>
              <select value={form.direction} onChange={(event) => update("direction", event.target.value as TraceDirection)}>
                <option value="forward">Forward (where did it go?)</option>
                <option value="backward">Backward (where did it come from?)</option>
              </select>
            </label>
            <label className="field">
              <span>Allocation</span>
              <select value={form.policy} onChange={(event) => update("policy", event.target.value as TracePolicy)}>
                <option value="fifo">FIFO</option>
                <option value="haircut">Haircut (proportional)</option>
                <option value="largest_out">Largest first</option>
              </select>
            </label>
            <label className="field">
              <span>Amount (optional)</span>
              <input type="number" min={0} step="any" value={form.amount} onChange={(event) => update("amount", event.target.value)} placeholder="All of it" />
            </label>
            <label className="field">
              <span>Asset</span>
              <input value={form.asset} onChange={(event) => update("asset", event.target.value)} placeholder="ETH.ETH" disabled={!amountSet} />
            </label>
            <label className="field">
              <span>Max hops</span>
              <input type="number" min={1} max={8} value={form.maxDepth} onChange={(event) => update("maxDepth", event.target.value)} />
            </label>
            <label className="field">
              <span>Max branches per address</span>
              <input type="number" min={1} max={64} value={form.maxBranches} onChange={(event) => update("maxBranches", event.target.value)} />
            </label>
            <label className="field">
              <span>Min USD (at time) to follow</span>
              <input type="number" min={0} step="any" value={form.minUSD} onChange={(event) => update("minUSD", event.target.value)} />
            </label>
            <fieldset className="field field-full trace-stops">
              <legend>Stop at labels</legend>
              {STOP_CATEGORIES.map((category) => (
                <label key={category} className="field-checkbox">
                  <input
                    type="checkbox"
                    checked={form.stopCategories.includes(category)}
                    onChange={(event) =>
                      update(
                        "stopCategories",
                        event.target.checked ? [...form.stopCategories, category] : form.stopCategories.filter((item) => item !== category)
                      )
                    }
                  />
                  <span>{category}</span>
                </label>
              ))}
            </fieldset>
            <label className="field-checkbox field-full">
              <input type="checkbox" checked={form.includeHoldings} onChange={(event) => update("includeHoldings", event.target.checked)} />
              <span>Look up what each endpoint holds now</span>
            </label>
            <div className="button-row field-full">
              <button type="submit" className="button" disabled={busy}>
                {busy ? "Tracing…" : "Trace"}
              </button>
            </div>
          </form>
          {status ? <p className="form-message">{status}</p> : null}
        </section>

        <section className="panel page-panel">
          <div className="panel-head">
            <div>
              <span className="eyebrow">History</span>
              <h2>Saved traces</h2>
            </div>
          </div>
          {runsQuery.data?.length ? (
            <div className="card-list">
              {runsQuery.data.map((run) => (
                <article key={run.id} className="entity-card">
                  <strong>{run.title}</strong>
                  <p>
                    {formatShortDateTime(run.created_at)} · {formatUSD(run.summary.seed_usd)} traced · {run.summary.sinks} sinks
                    {run.summary.top_sink ? ` · top: ${run.summary.top_sink}` : ""}
                  </p>
                  <div className="button-row">
                    <button type="button" className="button secondary" onClick={() => void onOpenRun(run.id)}>
                      Open
                    </button>
                    <button type="button" className="button secondary" onClick={() => void onDeleteRun(run.id)}>
                      Delete
                    </button>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <p className="empty-state">No saved traces yet.</p>
          )}
        </section>
      </div>
      {result ? <TraceResults result={result} /> : null}
    </div>
  );
}
