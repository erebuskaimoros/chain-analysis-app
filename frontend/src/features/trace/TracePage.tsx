import { useRef, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { PlusIcon, StopIcon, TrashIcon } from "../../app/icons";
import { useRouteIntent } from "../../app/router";
import { useToast } from "../../app/toast";
import { deleteTraceRun, getTraceRun, listTraceRuns, startTrace } from "../../lib/api";
import { isJobCanceled } from "../../lib/jobErrors";
import { formatCompactUSD, formatDateRange, formatRelativeTime, middleTruncate, pluralize } from "../../lib/format";
import { identifyInput } from "../../lib/identify";
import type { TraceDirection, TracePolicy, TraceRequest, TraceResponse, TraceRun, TraceSeed } from "../../lib/types";
import { MenuButton } from "../../ui/Menu";
import { PageHeader } from "../../ui/PageHeader";
import { Segmented } from "../../ui/Segmented";
import { TimeWindowField } from "../../ui/TimeWindowField";
import { TraceResults } from "./TraceResults";

const STOP_CATEGORIES = ["exchange", "sanctioned", "mixer", "scam", "hack", "bridge", "defi"];
const DEFAULT_STOPS = ["exchange", "sanctioned", "mixer"];

const POLICY_HELP: Record<TracePolicy, string> = {
  fifo: "Each payment spends the oldest funds received first.",
  haircut: "Each payment carries a proportional share of everything the address held.",
  largest_out: "Traced value follows the largest payments out first.",
};

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

function SeedChips({ text }: { text: string }) {
  const lines = text
    .split(/\n|,/)
    .map((line) => line.trim())
    .filter(Boolean);
  if (!lines.length) {
    return null;
  }
  return (
    <div className="seed-chips" aria-label="What each seed was read as">
      {lines.map((line, index) => {
        const identified = identifyInput(line);
        const known = identified.kind === "tx" || identified.kind === "address";
        const label =
          identified.kind === "tx"
            ? `Transaction ${middleTruncate(line, 6, 4)}`
            : identified.kind === "address"
              ? `${identified.chainCertain ? identified.chain : `${identified.chain}?`} ${middleTruncate(identified.value, 6, 4)}`
              : `Not recognised: ${middleTruncate(line, 10, 4)}`;
        return (
          <span key={`${line}:${index}`} className={`seed-chip${known ? "" : " is-unknown"}`} title={line}>
            {label}
          </span>
        );
      })}
    </div>
  );
}

function traceTitle(run: TraceRun) {
  return run.title || `Trace ${run.id}`;
}

export function TracePage() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const runsQuery = useQuery({ queryKey: ["trace-runs"], queryFn: listTraceRuns });
  const [form, setForm] = useState<TraceFormState>(defaultForm);
  const [result, setResult] = useState<TraceResponse | null>(null);
  const [status, setStatus] = useState("");
  const [progress, setProgress] = useState("");
  const [busy, setBusy] = useState(false);
  const runRef = useRef(0);
  const seedsRef = useRef<HTMLTextAreaElement | null>(null);

  function update<K extends keyof TraceFormState>(key: K, value: TraceFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  useRouteIntent("trace", (params) => {
    if (params.seed) {
      setForm((current) => ({ ...current, seeds: params.seed }));
      setResult(null);
      setStatus("Seed added. Check the window and direction, then trace.");
      window.setTimeout(() => seedsRef.current?.focus(), 0);
    }
    if (params.trace) {
      void onOpenRun(Number(params.trace));
    }
  });

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    const request = buildTraceRequest(form);
    if (!request.seeds.length) {
      setStatus("Enter at least one address or transaction hash.");
      return;
    }
    const runID = runRef.current + 1;
    runRef.current = runID;
    setBusy(true);
    setStatus("Tracing…");
    setProgress("Starting");
    try {
      const response = await startTrace(
        request,
        (job) => {
          if (runRef.current === runID) {
            const stage = job.stage || "starting";
            setProgress(`${stage.charAt(0).toUpperCase()}${stage.slice(1)}${job.message ? `: ${job.message}` : ""}`);
            setStatus(`Tracing: ${job.stage || "starting"}${job.message ? `, ${job.message}` : ""}…`);
          }
        },
        { isCanceled: () => runRef.current !== runID }
      );
      setResult(response);
      // The result's own summary says what was found.
      setStatus("");
      await queryClient.invalidateQueries({ queryKey: ["trace-runs"] });
    } catch (error) {
      setStatus(isJobCanceled(error) ? "Trace canceled." : error instanceof Error ? error.message : "Trace failed.");
    } finally {
      if (runRef.current === runID) {
        setBusy(false);
        setProgress("");
      }
    }
  }

  function cancelTrace() {
    runRef.current += 1;
    setBusy(false);
    setProgress("");
    setStatus("Trace canceled.");
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

  async function onDeleteRun(run: TraceRun) {
    await deleteTraceRun(run.id);
    await queryClient.invalidateQueries({ queryKey: ["trace-runs"] });
    toast(`Deleted ${traceTitle(run)}`);
  }

  function newTrace() {
    setResult(null);
    setForm(defaultForm());
    setStatus("");
    window.setTimeout(() => seedsRef.current?.focus(), 0);
  }

  const amountSet = Number(form.amount) > 0;
  const runs = runsQuery.data ?? [];
  const context = result
    ? `${result.query.direction === "backward" ? "Backward" : "Forward"} from ${pluralize(result.query.seeds.length, "seed")}, ${formatDateRange(
        result.query.start_time,
        result.query.end_time
      )}`
    : "";

  const recentList = runs.length ? (
    <ul className="list ws-run-list">
      {runs.map((run) => (
        <li key={run.id} className="list-row">
          <span className="list-row-main">
            <span className="list-row-title">{traceTitle(run)}</span>
            <span className="list-row-meta">
              {run.direction === "backward" ? "Backward" : "Forward"}, {formatCompactUSD(run.summary.seed_usd)} traced to{" "}
              {pluralize(run.summary.sinks, "endpoint")}
              {run.summary.top_sink ? `, most to ${run.summary.top_sink}` : ""}
            </span>
          </span>
          <span className="list-row-side">{formatRelativeTime(run.created_at)}</span>
          <button type="button" className="btn btn-sm" onClick={() => void onOpenRun(run.id)}>
            Open
          </button>
          <MenuButton
            label={`Actions for ${traceTitle(run)}`}
            items={[{ label: "Delete", icon: <TrashIcon />, danger: true, onSelect: () => void onDeleteRun(run) }]}
          />
        </li>
      ))}
    </ul>
  ) : (
    <div className="empty-state">
      <strong>No saved traces yet.</strong>
      <span>
        A trace follows value hop by hop from an address or transaction, through swaps across chains, to where it rests:
        exchanges, flagged addresses, pools, or wallets that still hold it. Every trace you run is saved here.
      </span>
    </div>
  );

  return (
    <>
      <PageHeader
        title="Trace"
        context={context}
        actions={
          <>
            {runs.length ? (
              <MenuButton
                label="Recent traces"
                className="btn btn-sm"
                items={runs.slice(0, 10).map((run) => ({
                  label: `${traceTitle(run)} (${formatRelativeTime(run.created_at)})`,
                  onSelect: () => void onOpenRun(run.id),
                }))}
              >
                Recent traces
              </MenuButton>
            ) : null}
            {result ? (
              <button type="button" className="btn btn-sm" onClick={newTrace}>
                <PlusIcon />
                New trace
              </button>
            ) : null}
          </>
        }
      />
      <div className="page-body trace-page">
        <form className="trace-query" onSubmit={(event) => void onSubmit(event)} aria-label="Trace settings">
          <div className="form-stack">
            <label className="field">
              <span>Seeds (one address, CHAIN|address, or transaction hash per line)</span>
              <textarea
                ref={seedsRef}
                className="mono"
                rows={3}
                value={form.seeds}
                onChange={(event) => update("seeds", event.target.value)}
                placeholder="ETH|0x… or a THORChain transaction hash"
                spellCheck={false}
              />
            </label>
            <SeedChips text={form.seeds} />
          </div>

          <div className="trace-query-side">
            <div className="field">
              <span>Direction</span>
              <Segmented<TraceDirection>
                label="Direction"
                block
                value={form.direction}
                onChange={(value) => update("direction", value)}
                options={[
                  { value: "forward", label: "Where did it go?", title: "Forward: follow value out of the seeds" },
                  { value: "backward", label: "Where did it come from?", title: "Backward: follow value into the seeds" },
                ]}
              />
            </div>
            <TimeWindowField
              utc
              start={form.startTime}
              end={form.endTime}
              onChange={(next) => setForm((current) => ({ ...current, startTime: next.start, endTime: next.end }))}
            />
            <div className="form-actions">
              {busy ? (
                <button type="button" className="btn" onClick={cancelTrace}>
                  <StopIcon />
                  Cancel
                </button>
              ) : (
                <button type="submit" className="btn btn-primary">
                  Trace
                </button>
              )}
            </div>
          </div>

          <details className="disclosure">
            <summary>More options</summary>
            <div className="disclosure-body trace-options">
              <div className="field field-wide">
                <span>Allocation</span>
                <Segmented<TracePolicy>
                  label="Allocation"
                  value={form.policy}
                  onChange={(value) => update("policy", value)}
                  options={[
                    { value: "fifo", label: "FIFO" },
                    { value: "haircut", label: "Haircut (proportional)" },
                    { value: "largest_out", label: "Largest first" },
                  ]}
                />
                <small>{POLICY_HELP[form.policy]}</small>
              </div>
              <label className="field">
                <span>Amount (optional)</span>
                <input
                  type="number"
                  min={0}
                  step="any"
                  value={form.amount}
                  onChange={(event) => update("amount", event.target.value)}
                  placeholder="All of it"
                />
              </label>
              <label className="field">
                <span>Asset</span>
                <input
                  value={form.asset}
                  onChange={(event) => update("asset", event.target.value)}
                  placeholder="ETH.ETH"
                  disabled={!amountSet}
                />
              </label>
              <label className="field">
                <span>Min USD (at time) to follow</span>
                <input
                  type="number"
                  min={0}
                  step="any"
                  value={form.minUSD}
                  onChange={(event) => update("minUSD", event.target.value)}
                />
              </label>
              <label className="field">
                <span>Max hops</span>
                <input
                  type="number"
                  min={1}
                  max={8}
                  value={form.maxDepth}
                  onChange={(event) => update("maxDepth", event.target.value)}
                />
              </label>
              <label className="field">
                <span>Max branches per address</span>
                <input
                  type="number"
                  min={1}
                  max={64}
                  value={form.maxBranches}
                  onChange={(event) => update("maxBranches", event.target.value)}
                />
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={form.includeHoldings}
                  onChange={(event) => update("includeHoldings", event.target.checked)}
                />
                <span>Look up what each endpoint holds now</span>
              </label>
              <fieldset className="field field-wide trace-stops" style={{ border: 0, padding: 0, margin: 0 }}>
                <legend className="field-label">Stop at labels</legend>
                <div className="chip-set">
                  {STOP_CATEGORIES.map((category) => (
                    <label key={category} className="toggle-chip">
                      <input
                        type="checkbox"
                        className="visually-hidden"
                        checked={form.stopCategories.includes(category)}
                        onChange={(event) =>
                          update(
                            "stopCategories",
                            event.target.checked
                              ? [...form.stopCategories, category]
                              : form.stopCategories.filter((item) => item !== category)
                          )
                        }
                      />
                      <span>{category}</span>
                    </label>
                  ))}
                </div>
                <small>The trace stops when value reaches an address with one of these labels.</small>
              </fieldset>
            </div>
          </details>
        </form>

        {status ? (
          <p className="status-text" role="status">
            {status}
          </p>
        ) : null}

        {busy ? (
          <div className="trace-progress" aria-live="polite">
            <span className="section-title">{progress || "Starting"}</span>
            <div className="progress is-indeterminate">
              <div className="progress-bar" />
            </div>
            <span className="section-note">Traces fetch each address's history as they go, so the first run can take a while.</span>
          </div>
        ) : null}

        {result ? (
          <TraceResults result={result} />
        ) : (
          <section className="section" aria-labelledby="trace-recent-title">
            <div className="section-head">
              <h2 id="trace-recent-title">Saved traces</h2>
            </div>
            {recentList}
          </section>
        )}
      </div>
    </>
  );
}
