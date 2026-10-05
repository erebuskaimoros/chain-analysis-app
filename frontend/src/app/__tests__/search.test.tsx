import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ActiveCaseProvider } from "../activeCase";
import { RouterProvider } from "../router";
import { Omnibox } from "../search";
import { ToastProvider } from "../toast";

const apiMocks = vi.hoisted(() => ({
  listActors: vi.fn(),
  listAnnotations: vi.fn(),
  listCases: vi.fn(),
  addCaseItem: vi.fn(),
  deleteCaseItem: vi.fn(),
  createCase: vi.fn(),
}));

vi.mock("../../lib/api", () => apiMocks);

const THOR = `thor1${"q".repeat(38)}`;
const TX = "A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F";

function renderOmnibox() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider>
        <ToastProvider>
          <ActiveCaseProvider>
            <Omnibox />
          </ActiveCaseProvider>
        </ToastProvider>
      </RouterProvider>
    </QueryClientProvider>
  );
}

describe("search", () => {
  beforeEach(() => {
    apiMocks.listActors.mockResolvedValue([
      { id: 3, name: "TC Treasury", color: "#00ff00", notes: "", addresses: [], created_at: "", updated_at: "" },
    ]);
    apiMocks.listAnnotations.mockResolvedValue([
      { id: 1, address: THOR, normalized_address: THOR, kind: "label", value: "Treasury Hot Wallet", created_at: "" },
    ]);
    apiMocks.listCases.mockResolvedValue([
      { id: 4, title: "Bitget hack", notes_md: "", created_at: "", updated_at: "", item_count: 0 },
    ]);
    apiMocks.addCaseItem.mockResolvedValue({ id: 11, case_id: 4, kind: "tx", ref: TX, note: "", pinned_at: "" });
    apiMocks.deleteCaseItem.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    window.localStorage.clear();
    window.location.hash = "";
  });

  it("offers what you can do with a pasted address and runs the first with Enter", () => {
    renderOmnibox();
    const input = screen.getByRole("combobox", { name: /Search addresses/ });
    fireEvent.change(input, { target: { value: THOR } });

    const results = screen.getByRole("listbox", { name: "Search results" });
    expect(within(results).getByRole("option", { name: /Explore this address/ })).toBeTruthy();
    expect(within(results).getByRole("option", { name: /Trace funds from this address/ })).toBeTruthy();
    expect(within(results).getByRole("option", { name: /Add to the active case/ })).toBeTruthy();

    fireEvent.keyDown(input, { key: "Enter" });
    expect(window.location.hash).toBe(`#explorer?address=${THOR}`);
  });

  it("finds actors, labeled addresses and cases by name", async () => {
    renderOmnibox();
    const input = screen.getByRole("combobox", { name: /Search addresses/ });
    fireEvent.change(input, { target: { value: "treas" } });

    expect(await screen.findByRole("option", { name: /Graph TC Treasury/ })).toBeTruthy();
    expect(await screen.findByRole("option", { name: /Treasury Hot Wallet/ })).toBeTruthy();

    fireEvent.change(input, { target: { value: "bitget" } });
    expect(await screen.findByRole("option", { name: /Bitget hack/ })).toBeTruthy();
  });

  it("asks which case to use when none is active, then adds the item with an undo", async () => {
    renderOmnibox();
    const input = screen.getByRole("combobox", { name: /Search addresses/ });
    fireEvent.change(input, { target: { value: TX } });
    fireEvent.click(screen.getByRole("option", { name: /Add to the active case/ }));

    const dialog = await screen.findByRole("dialog");
    fireEvent.click(await within(dialog).findByRole("radio", { name: /Bitget hack/ }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Add to case" }));

    await waitFor(() => expect(apiMocks.addCaseItem).toHaveBeenCalledWith(4, "tx", TX, ""));
    expect(window.localStorage.getItem("chain-analysis.active-case")).toBe("4");
    const toast = await screen.findByText(/Added .* to Bitget hack/);
    fireEvent.click(within(toast.parentElement as HTMLElement).getByRole("button", { name: "Undo" }));
    await waitFor(() => expect(apiMocks.deleteCaseItem).toHaveBeenCalledWith(4, 11));
  });
});
