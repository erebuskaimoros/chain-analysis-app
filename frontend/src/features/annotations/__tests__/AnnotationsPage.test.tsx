import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AnnotationsPage } from "../AnnotationsPage";

const apiMocks = vi.hoisted(() => ({
  listAnnotations: vi.fn(),
  upsertAnnotation: vi.fn(),
  deleteAnnotation: vi.fn(),
  listBlocklist: vi.fn(),
  addToBlocklist: vi.fn(),
  removeFromBlocklist: vi.fn(),
}));

vi.mock("../../../lib/api", () => ({
  listAnnotations: apiMocks.listAnnotations,
  upsertAnnotation: apiMocks.upsertAnnotation,
  deleteAnnotation: apiMocks.deleteAnnotation,
  listBlocklist: apiMocks.listBlocklist,
  addToBlocklist: apiMocks.addToBlocklist,
  removeFromBlocklist: apiMocks.removeFromBlocklist,
}));

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <AnnotationsPage />
    </QueryClientProvider>
  );
}

describe("AnnotationsPage", () => {
  afterEach(() => {
    cleanup();
    Object.values(apiMocks).forEach((mockFn) => mockFn.mockReset());
  });

  const treasury = {
    id: 1,
    address: "thor1treasury",
    normalized_address: "thor1treasury",
    kind: "label",
    value: "Treasury Hot Wallet",
    created_at: "2026-03-11T12:00:00Z",
  };

  it("edits an existing label and changes only its value", async () => {
    apiMocks.listAnnotations.mockResolvedValue([treasury]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.upsertAnnotation.mockResolvedValue({ ok: true });

    renderPage();

    await screen.findByText("Treasury Hot Wallet");
    fireEvent.click(screen.getByRole("button", { name: "Actions for Treasury Hot Wallet" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit" }));

    const addressInput = screen.getByPlaceholderText("thor1...") as HTMLInputElement;
    const kindInput = screen.getByLabelText("Kind") as HTMLInputElement;
    const valueInput = screen.getByPlaceholderText("Treasury hot wallet") as HTMLInputElement;

    expect(addressInput.value).toBe("thor1treasury");
    expect(kindInput.value).toBe("label");
    expect(valueInput.value).toBe("Treasury Hot Wallet");
    expect(addressInput.disabled).toBe(true);
    expect(kindInput.disabled).toBe(true);

    fireEvent.change(valueInput, { target: { value: "Treasury Cold Wallet" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(apiMocks.upsertAnnotation).toHaveBeenCalled());
    expect(apiMocks.upsertAnnotation.mock.calls[0]?.[0]).toEqual({
      address: "thor1treasury",
      kind: "label",
      value: "Treasury Cold Wallet",
    });
  });

  it("cancels editing and starts the next label from a blank form", async () => {
    apiMocks.listAnnotations.mockResolvedValue([treasury]);
    apiMocks.listBlocklist.mockResolvedValue([]);

    renderPage();

    await screen.findByText("Treasury Hot Wallet");
    fireEvent.click(screen.getByRole("button", { name: "Actions for Treasury Hot Wallet" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "New label" }));
    expect((screen.getByPlaceholderText("thor1...") as HTMLInputElement).value).toBe("");
    expect((screen.getByLabelText("Kind") as HTMLSelectElement).value).toBe("label");
    expect((screen.getByPlaceholderText("Treasury hot wallet") as HTMLInputElement).value).toBe("");
    expect(screen.getByRole("button", { name: "Save label" })).toBeTruthy();
  });

  it("asks before deleting a label", async () => {
    apiMocks.listAnnotations.mockResolvedValue([treasury]);
    apiMocks.listBlocklist.mockResolvedValue([]);
    apiMocks.deleteAnnotation.mockResolvedValue({ ok: true });

    renderPage();

    await screen.findByText("Treasury Hot Wallet");
    fireEvent.click(screen.getByRole("button", { name: "Actions for Treasury Hot Wallet" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete…" }));
    expect(screen.getByRole("dialog", { name: "Delete this label?" })).toBeTruthy();
    expect(apiMocks.deleteAnnotation).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Delete label" }));
    await waitFor(() => expect(apiMocks.deleteAnnotation).toHaveBeenCalled());
    expect(apiMocks.deleteAnnotation.mock.calls[0]?.[0]).toEqual({ address: "thor1treasury", kind: "label" });
  });
});
