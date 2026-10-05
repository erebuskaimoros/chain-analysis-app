import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TraceResponse } from "../../../lib/types";
import { ActiveCaseProvider } from "../../../app/activeCase";
import { ToastProvider } from "../../../app/toast";
import { buildTraceRequest, parseTraceSeeds, TracePage, type TraceFormState } from "../TracePage";

const apiMocks = vi.hoisted(() => ({
  startTrace: vi.fn(),
  listTraceRuns: vi.fn(),
  getTraceRun: vi.fn(),
  deleteTraceRun: vi.fn(),
  listCases: vi.fn(),
  addCaseItem: vi.fn(),
}));

vi.mock("../../../lib/api", () => apiMocks);

vi.mock("../../shared/GraphCanvas", () => ({
  GraphCanvas: ({ nodes, edges }: { nodes: unknown[]; edges: unknown[] }) => (
    <div data-testid="graph-canvas-mock">
      {nodes.length} nodes, {edges.length} edges
    </div>
  ),
}));

const seed = "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3";
const destination = "BTC|bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f";

function makeTrace(): TraceResponse {
  return {
    run_id: 7,
    query: {
      seeds: [{ chain: "ETH", address: "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3" }],
      start_time: "2026-09-28T03:00:00Z",
      end_time: "2026-09-28T07:00:00Z",
      direction: "forward",
      policy: "fifo",
      max_depth: 1,
      max_branches: 8,
      min_usd_at_time: 0,
      stop_categories: ["exchange", "sanctioned", "mixer"],
    },
    nodes: [
      { id: seed, kind: "external_address", label: "Exploiter ETH", chain: "ETH", stage: "", depth: 0, actor_ids: [], shared: false, collapsed: false, metrics: { address: "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3" } },
      { id: destination, kind: "external_address", label: "bc1qvq…cqj68f", chain: "BTC", stage: "", depth: 1, actor_ids: [], shared: false, collapsed: false, metrics: { address: "bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f" } },
    ],
    edges: [
      {
        id: `${seed}->${destination}|midgard.swap`,
        from: seed,
        to: destination,
        action_class: "swaps",
        action_key: "midgard.swap",
        action_label: "Swap",
        action_domain: "swaps",
        validator_address: "",
        validator_label: "",
        contract_type: "",
        contract_protocol: "",
        assets: [],
        transactions: [],
        usd_spot: 2655,
        usd_at_time: 2655,
        tx_ids: ["9CD94A8E5734DD6E4C77D0EB400F1C64935BE17AFC062185CB26264004E55CA9"],
        heights: [],
        actor_ids: [],
        confidence: 1,
        depth: 1,
        traced_assets: [{ asset: "BTC.BTC", amount: 0.0316689, usd_at_time: 2641 }],
        traced_input_assets: [{ asset: "ETH.ETH", amount: 1, usd_at_time: 2655 }],
        traced_usd_at_time: 2655,
        confidence_reason: "THORChain swap links the deposit to the payout",
        traced_transactions: [
          {
            tx_id: "9CD94A8E5734DD6E4C77D0EB400F1C64935BE17AFC062185CB26264004E55CA9",
            inbound_tx_id: "A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F",
            time: "2026-09-28T03:55:51Z",
            asset: "BTC.BTC",
            amount: 0.0316689,
            traced_amount: 0.0316689,
            input_asset: "ETH.ETH",
            input_amount: 1,
            traced_input_amount: 1,
            traced_usd_at_time: 2655,
            priced: true,
            fraction: 1,
            confidence: 1,
          },
        ],
      },
    ],
    sinks: [],
    frontier: [
      {
        node_id: destination,
        chain: "BTC",
        address: "bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f",
        label: "bc1qvq…cqj68f",
        kind: "address",
        reason: "max_depth",
        depth: 1,
        assets: [{ asset: "BTC.BTC", amount: 0.0316689, usd_at_time: 2641 }],
        traced_usd_at_time: 2641,
        confidence: 1,
        first_at: "2026-09-28T03:55:51Z",
        last_at: "2026-09-28T03:55:51Z",
      },
    ],
    coverage_gaps: [],
    warnings: [],
    method: ["Forward trace from 1 seed address(es), 2026-09-28 03:00 to 2026-09-28 07:00 UTC.", "FIFO: each payment spends the oldest funds received first."],
    totals: { seed_assets: [{ asset: "ETH.ETH", amount: 1, usd_at_time: 2655 }], seed_usd: 2655, sink_usd: 0, frontier_usd: 2641 },
    stats: {},
  } as TraceResponse;
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <ActiveCaseProvider>
          <TracePage />
        </ActiveCaseProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}

describe("trace request helpers", () => {
  it("reads addresses, chain-qualified addresses and transaction hashes", () => {
    expect(
      parseTraceSeeds("ETH|0xabc\nthor1xyz\n0xA732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F\n\n")
    ).toEqual([
      { chain: "ETH", address: "0xabc" },
      { address: "thor1xyz" },
      { tx_id: "0xA732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F" },
    ]);
  });

  it("sends times as UTC and only sends an asset with an amount", () => {
    const form: TraceFormState = {
      seeds: "BTC|bc1qx",
      startTime: "2026-09-28T03:00",
      endTime: "2026-09-28T07:00",
      direction: "backward",
      policy: "haircut",
      amount: "",
      asset: "BTC.BTC",
      maxDepth: "2",
      maxBranches: "4",
      minUSD: "100",
      stopCategories: ["exchange"],
      includeHoldings: false,
    };
    const request = buildTraceRequest(form);
    expect(request.start_time).toBe("2026-09-28T03:00:00Z");
    expect(request.end_time).toBe("2026-09-28T07:00:00Z");
    expect(request.asset).toBeUndefined();
    expect(request).toMatchObject({ direction: "backward", policy: "haircut", max_depth: 2, max_branches: 4, min_usd_at_time: 100 });
    expect(buildTraceRequest({ ...form, amount: "1.5", asset: "btc.btc" })).toMatchObject({ amount: 1.5, asset: "BTC.BTC" });
  });
});

describe("TracePage", () => {
  beforeEach(() => {
    window.localStorage.setItem("chain-analysis.active-case", "4");
    apiMocks.listTraceRuns.mockResolvedValue([]);
    apiMocks.startTrace.mockResolvedValue(makeTrace());
    apiMocks.listCases.mockResolvedValue([{ id: 4, title: "Bitget hack", notes_md: "", created_at: "", updated_at: "", item_count: 0 }]);
    apiMocks.addCaseItem.mockResolvedValue({});
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    window.localStorage.clear();
  });

  it("runs a trace and shows the endpoints, flows with both hashes, and the method", async () => {
    renderPage();
    await screen.findByText("No saved traces yet.");
    fireEvent.change(screen.getByLabelText(/^Seeds/), { target: { value: seed } });
    fireEvent.click(screen.getByRole("button", { name: "Trace" }));
    await waitFor(() => expect(apiMocks.startTrace).toHaveBeenCalled());
    expect(apiMocks.startTrace.mock.calls[0][0].seeds).toEqual([{ chain: "ETH", address: "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3" }]);

    const limits = (await screen.findByRole("heading", { name: /^Stopped by limits/ })).closest("section") as HTMLElement;
    expect(within(limits).getByText("Hop limit")).toBeTruthy();
    expect(within(limits).getByText("0.0316689 BTC.BTC")).toBeTruthy();

    const flows = screen.getByRole("heading", { name: /^Flows/ }).closest("section") as HTMLElement;
    const links = within(flows).getAllByRole("link").map((link) => link.getAttribute("href"));
    expect(links).toContain("https://etherscan.io/tx/0xa732e09aab76a571d768c2b5b4e2f0e1e5b1a9c3d4e5f60718293a4b5c6d7e8f");
    expect(links).toContain("https://mempool.space/tx/9cd94a8e5734dd6e4c77d0eb400f1c64935be17afc062185cb26264004e55ca9");
    expect(within(flows).getByText(/1 ETH\.ETH → 0\.0316689 BTC\.BTC/)).toBeTruthy();
    expect(screen.getByText("FIFO: each payment spends the oldest funds received first.")).toBeTruthy();
    expect(screen.getByTestId("graph-canvas-mock").textContent).toContain("2 nodes, 1 edges");
    await waitFor(() => expect(apiMocks.listTraceRuns).toHaveBeenCalledTimes(2));

    // With a case active, the trace files straight into it and says so.
    fireEvent.click(await screen.findByRole("button", { name: "Add trace to case" }));
    await waitFor(() => expect(apiMocks.addCaseItem).toHaveBeenCalledWith(4, "trace_run", "7", ""));
    expect(await screen.findByText("Added trace 7 to Bitget hack")).toBeTruthy();
  });

  it("refuses to start without seeds", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Trace" }));
    expect(await screen.findByText("Enter at least one address or transaction hash.")).toBeTruthy();
    expect(apiMocks.startTrace).not.toHaveBeenCalled();
  });

  it("opens a saved trace", async () => {
    apiMocks.listTraceRuns.mockResolvedValue([
      {
        id: 7,
        created_at: "2026-10-05T06:00:00Z",
        title: "Forward FIFO trace from 0xf7bc…96c3",
        direction: "forward",
        policy: "fifo",
        request: { seeds: [{ chain: "ETH", address: "0xf7bc" }], start_time: "2026-09-28T03:00:00Z", max_depth: 1 },
        summary: { seeds: [], seed_usd: 2655, sink_usd: 0, frontier_usd: 2641, sinks: 0, frontier: 1, edges: 1 },
      },
    ]);
    apiMocks.getTraceRun.mockResolvedValue({ id: 7, title: "Forward FIFO trace", request: { seeds: [{ chain: "ETH", address: "0xf7bc" }], start_time: "2026-09-28T03:00:00Z", max_depth: 1 }, response: makeTrace() });
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Open" }));
    expect(await screen.findByRole("heading", { name: /^Flows/ })).toBeTruthy();
    expect((screen.getByLabelText(/^Seeds/) as HTMLTextAreaElement).value).toBe("ETH|0xf7bc");
    expect((screen.getByLabelText("Max hops") as HTMLInputElement).value).toBe("1");
  });
});
