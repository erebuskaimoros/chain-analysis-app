import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeActor } from "../../../test-support/graphFixtures";
import type { ActorMonitor } from "../../../lib/types";
import { ActorMonitorPanel } from "../ActorMonitorPanel";
import { HoldingsChart } from "../HoldingsChart";

const apiMocks = vi.hoisted(() => ({
  getActorMonitor: vi.fn(),
  setActorWatch: vi.fn(),
  markActorViewed: vi.fn(),
  refreshActor: vi.fn(),
}));

vi.mock("../../../lib/api", () => apiMocks);

function makeMonitor(overrides: Partial<ActorMonitor> = {}): ActorMonitor {
  return {
    actor_id: 1,
    watch: false,
    last_viewed_at: "",
    series: [
      { taken_at: "2026-10-01T00:00:00Z", total_usd: 1_000_000 },
      { taken_at: "2026-10-02T00:00:00Z", total_usd: 940_000 },
    ],
    latest: {
      id: 2,
      actor_id: 1,
      taken_at: "2026-10-02T00:00:00Z",
      window_start: "2026-10-01T00:00:00Z",
      window_end: "2026-10-02T00:00:00Z",
      total_usd: 940_000,
      baseline: false,
      holdings: [],
      flows: { in_usd: 0, out_usd: 60_000, transactions: 1, counterparties: [] },
    },
    since_last_view: {
      since: "",
      snapshots: 1,
      total_usd_before: 1_000_000,
      total_usd_after: 940_000,
      holding_deltas: [{ chain: "THOR", address: "thor1treasury", label: "Treasury hot", before_usd: 1_000_000, after_usd: 940_000, delta_usd: -60_000 }],
      asset_deltas: [{ asset: "THOR.RUNE", before_amount: 500_000, after_amount: 470_000, before_usd: 1_000_000, after_usd: 940_000, delta_usd: -60_000 }],
      flows: {
        in_usd: 0,
        out_usd: 60_000,
        transactions: 1,
        counterparties: [
          {
            key: "THOR|thor1vendor",
            label: "Binance hot wallet",
            kind: "external_address",
            category: "exchange",
            in_usd: 0,
            out_usd: 60_000,
            transactions: 1,
            action_classes: ["transfers"],
            first_seen: true,
          },
        ],
      },
      new_counterparties: [
        {
          key: "THOR|thor1vendor",
          label: "Binance hot wallet",
          kind: "external_address",
          category: "exchange",
          in_usd: 0,
          out_usd: 60_000,
          transactions: 1,
          action_classes: ["transfers"],
          first_seen: true,
        },
      ],
    },
    ...overrides,
  };
}

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ActorMonitorPanel actor={makeActor({ name: "TC Treasury" })} />
    </QueryClientProvider>,
  );
}

describe("ActorMonitorPanel", () => {
  beforeEach(() => {
    apiMocks.getActorMonitor.mockResolvedValue(makeMonitor());
    apiMocks.setActorWatch.mockResolvedValue(undefined);
    apiMocks.markActorViewed.mockResolvedValue(undefined);
    apiMocks.refreshActor.mockResolvedValue(makeMonitor().latest);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("summarises what changed since the last look and highlights large moves", async () => {
    renderPanel();
    expect((await screen.findByText(/since you last looked/)).textContent).toContain("−$60,000 since you last looked");
    const assetTable = screen.getByRole("columnheader", { name: "Asset" }).closest("table")!;
    const runeRow = within(assetTable).getByText("THOR.RUNE").closest("tr")!;
    expect(runeRow.classList.contains("row-highlight")).toBe(true);
    expect(within(runeRow).getByText("500,000")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Highlight moves ≥ USD"), { target: { value: "100000" } });
    expect(runeRow.classList.contains("row-highlight")).toBe(false);

    const newList = screen.getByRole("heading", { name: "New counterparties" }).nextElementSibling as HTMLElement;
    expect(within(newList).getByText("exchange").classList.contains("label-category-exchange")).toBe(true);
  });

  it("refreshes, toggles watching and marks changes as seen", async () => {
    apiMocks.refreshActor.mockImplementation(async (_id: number, onProgress: (job: { stage: string }) => void) => {
      onProgress({ stage: "flows" });
      return makeMonitor().latest;
    });
    renderPanel();
    await screen.findByText(/since you last looked/);

    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    await waitFor(() => expect(apiMocks.refreshActor).toHaveBeenCalledWith(1, expect.any(Function)));
    await waitFor(() => expect(apiMocks.getActorMonitor).toHaveBeenCalledTimes(2));
    expect(screen.getByText(/^Refreshed /)).toBeTruthy();

    fireEvent.click(screen.getByRole("checkbox", { name: "Refresh on schedule" }));
    await waitFor(() => expect(apiMocks.setActorWatch).toHaveBeenCalledWith(1, true));

    fireEvent.click(screen.getByRole("button", { name: "Mark as seen" }));
    await waitFor(() => expect(apiMocks.markActorViewed).toHaveBeenCalledWith(1));
  });

  it("shows an empty state before the first refresh", async () => {
    apiMocks.getActorMonitor.mockResolvedValue(makeMonitor({ series: [], latest: undefined, since_last_view: undefined }));
    renderPanel();
    expect(await screen.findByText("No refreshes yet. Refresh to record the first snapshot.")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Mark as seen" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("HoldingsChart", () => {
  afterEach(() => cleanup());

  const points = [
    { taken_at: "2026-10-01T00:00:00Z", total_usd: 1_000_000 },
    { taken_at: "2026-10-02T00:00:00Z", total_usd: 940_000 },
    { taken_at: "2026-10-03T00:00:00Z", total_usd: 1_250_000 },
  ];

  it("moves the crosshair with the keyboard and names the focused value", () => {
    render(<HoldingsChart points={points} />);
    const chart = screen.getByRole("img", { name: /3 refreshes, latest \$1,250,000/ });
    fireEvent.focus(chart);
    expect(screen.getByRole("status").textContent).toContain("$1,250,000");
    fireEvent.keyDown(chart, { key: "ArrowLeft" });
    expect(screen.getByRole("status").textContent).toContain("$940,000");
    fireEvent.blur(chart);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("offers a table view with every value", () => {
    render(<HoldingsChart points={points} />);
    fireEvent.click(screen.getByRole("button", { name: "Show table" }));
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(4);
    expect(rows[3].textContent).toContain("$1,250,000");
  });
});
