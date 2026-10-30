import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RouterProvider } from "../../../app/router";
import type { Actor } from "../../../lib/types";
import { ActorsPage } from "../ActorsPage";

const apiMocks = vi.hoisted(() => ({
  listActors: vi.fn(),
  listAnnotations: vi.fn(),
  createActor: vi.fn(),
  updateActor: vi.fn(),
  deleteActor: vi.fn(),
  getActorMonitor: vi.fn(),
  setActorWatch: vi.fn(),
  markActorViewed: vi.fn(),
  refreshActor: vi.fn(),
}));

vi.mock("../../../lib/api", () => apiMocks);

const BCH = "qz7262r7d0tvv4kfqfv8kgaykflm4t5xcu6c7vskzw";
const THOR = `thor1${"q".repeat(38)}`;

const treasury: Actor = {
  id: 3,
  name: "TC Treasury",
  color: "#2a78d6",
  notes: "Protocol treasury",
  created_at: "",
  updated_at: "",
  addresses: [
    { id: 1, actor_id: 3, address: THOR, chain_hint: "THOR", label: "main", normalized_address: THOR, created_at: "" },
    { id: 2, actor_id: 3, address: BCH, chain_hint: "", label: "bch", normalized_address: BCH, created_at: "" },
  ],
};

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider>
        <ActorsPage />
      </RouterProvider>
    </QueryClientProvider>
  );
}

describe("ActorsPage", () => {
  beforeEach(() => {
    apiMocks.listActors.mockResolvedValue([treasury]);
    apiMocks.listAnnotations.mockResolvedValue([]);
    apiMocks.getActorMonitor.mockResolvedValue({ actor_id: 3, watch: false, last_viewed_at: "", series: [] });
    apiMocks.updateActor.mockResolvedValue(treasury);
    apiMocks.createActor.mockResolvedValue({ ...treasury, id: 9, name: "New actor" });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    window.location.hash = "";
  });

  it("shows the first actor's addresses grouped by chain", async () => {
    renderPage();
    const details = await screen.findByRole("region", { name: "Actor details" });
    expect(await within(details).findByRole("heading", { name: "TC Treasury" })).toBeTruthy();
    const addresses = within(details).getByRole("region", { name: "Addresses" });
    expect(within(addresses).getByText("main")).toBeTruthy();
    expect(within(addresses).getByText("bch")).toBeTruthy();
  });

  it("offers to set the chain on addresses that are missing one", async () => {
    renderPage();
    const notice = await screen.findByRole("note");
    expect(notice.textContent).toContain("Detected: BCH");

    fireEvent.click(within(notice).getByRole("button", { name: "Set the detected chains" }));

    await waitFor(() => expect(apiMocks.updateActor).toHaveBeenCalled());
    const [id, payload] = apiMocks.updateActor.mock.calls[0];
    expect(id).toBe(3);
    expect(payload.addresses).toEqual([
      { address: THOR, chain_hint: "THOR", label: "main" },
      { address: BCH, chain_hint: "BCH", label: "bch" },
    ]);
  });

  it("fills in the chain when a recognised address is typed", async () => {
    renderPage();
    await screen.findByRole("heading", { name: "TC Treasury" });
    fireEvent.click(screen.getByRole("button", { name: "New actor" }));

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Bitget exploiter" } });
    const address = screen.getByLabelText("Address 1");
    fireEvent.change(address, { target: { value: BCH } });
    fireEvent.blur(address);
    expect((screen.getByLabelText("Chain for address 1") as HTMLSelectElement).value).toBe("BCH");

    fireEvent.click(screen.getByRole("button", { name: "Create actor" }));
    await waitFor(() => expect(apiMocks.createActor).toHaveBeenCalled());
    expect(apiMocks.createActor.mock.calls[0][0]).toMatchObject({
      name: "Bitget exploiter",
      addresses: [{ address: BCH, chain_hint: "BCH", label: "" }],
    });
  });

  it("graphs an actor straight from its page", async () => {
    renderPage();
    await screen.findByRole("heading", { name: "TC Treasury" });
    fireEvent.click(screen.getByRole("button", { name: "Graph this actor" }));
    expect(window.location.hash).toBe("#graph?actors=3&run=1");
  });
});
