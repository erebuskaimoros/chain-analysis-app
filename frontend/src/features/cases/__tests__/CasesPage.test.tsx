import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Case } from "../../../lib/types";
import { CasesPage } from "../CasesPage";

const apiMocks = vi.hoisted(() => ({
  listCases: vi.fn(),
  getCase: vi.fn(),
  createCase: vi.fn(),
  updateCase: vi.fn(),
  deleteCase: vi.fn(),
  addCaseItem: vi.fn(),
  deleteCaseItem: vi.fn(),
  caseExportURL: (id: number, format: string) => `/api/v1/cases/${id}/export?format=${format}`,
}));

vi.mock("../../../lib/api", () => apiMocks);

const bitget: Case = {
  id: 4,
  title: "Bitget hack",
  notes_md: "Reported 2026-09-28.",
  created_at: "2026-10-05T09:00:00Z",
  updated_at: "2026-10-05T09:30:00Z",
  item_count: 2,
  items: [
    { id: 1, case_id: 4, kind: "address", ref: "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3", note: "exploiter", pinned_at: "", title: "Exploiter" },
    { id: 2, case_id: 4, kind: "trace_run", ref: "7", note: "", pinned_at: "", title: "Forward FIFO trace" },
  ],
};

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <CasesPage />
    </QueryClientProvider>
  );
}

describe("CasesPage", () => {
  beforeEach(() => {
    apiMocks.listCases.mockResolvedValue([bitget]);
    apiMocks.getCase.mockResolvedValue(bitget);
    apiMocks.createCase.mockResolvedValue({ ...bitget, id: 5, title: "New case", items: [] });
    apiMocks.updateCase.mockResolvedValue(bitget);
    apiMocks.addCaseItem.mockResolvedValue({});
    apiMocks.deleteCaseItem.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("opens a case with its items, explorer links and exports", async () => {
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Open" }));
    const details = await screen.findByRole("region", { name: "Case details" });
    expect(await within(details).findByRole("heading", { name: "Bitget hack" })).toBeTruthy();
    const address = within(details).getByRole("link", { name: /Exploiter/ });
    expect(address.getAttribute("href")).toBe("https://etherscan.io/address/0xf7bc92103f23ef312658cd9b81dc2713f7b396c3");
    expect(within(details).getByText("Saved trace 7: Forward FIFO trace")).toBeTruthy();
    expect(within(details).getByRole("link", { name: "Export Markdown" }).getAttribute("href")).toBe("/api/v1/cases/4/export?format=md");
    expect((within(details).getByLabelText("Notes (Markdown)") as HTMLTextAreaElement).value).toBe("Reported 2026-09-28.");
  });

  it("pins an item, saves notes and removes an item", async () => {
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Open" }));
    const details = await screen.findByRole("region", { name: "Case details" });
    await within(details).findByRole("heading", { name: "Bitget hack" });

    fireEvent.change(within(details).getByLabelText("Pin"), { target: { value: "tx" } });
    fireEvent.change(within(details).getByLabelText("Reference"), { target: { value: "0xa732" } });
    fireEvent.click(within(details).getByRole("button", { name: "Pin item" }));
    await waitFor(() => expect(apiMocks.addCaseItem).toHaveBeenCalledWith(4, "tx", "0xa732", ""));

    fireEvent.change(within(details).getByLabelText("Notes (Markdown)"), { target: { value: "Updated." } });
    fireEvent.click(within(details).getByRole("button", { name: "Save notes" }));
    await waitFor(() => expect(apiMocks.updateCase).toHaveBeenCalledWith(4, "Bitget hack", "Updated."));

    fireEvent.click(within(details).getAllByRole("button", { name: "Remove" })[0]);
    await waitFor(() => expect(apiMocks.deleteCaseItem).toHaveBeenCalledWith(4, 1));
  });

  it("creates a case and opens it", async () => {
    renderPage();
    await screen.findByRole("button", { name: "Open" });
    fireEvent.change(screen.getByLabelText("New case title"), { target: { value: "New case" } });
    fireEvent.click(screen.getByRole("button", { name: "Create case" }));
    await waitFor(() => expect(apiMocks.createCase).toHaveBeenCalledWith("New case"));
    await waitFor(() => expect(apiMocks.getCase).toHaveBeenCalledWith(5));
  });
});
