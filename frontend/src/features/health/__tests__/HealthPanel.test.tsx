import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HealthPanel } from "../HealthPanel";

const apiMocks = vi.hoisted(() => ({
  getHealth: vi.fn(),
}));

vi.mock("../../../lib/api", () => ({
  getHealth: apiMocks.getHealth,
}));

describe("HealthPanel", () => {
  afterEach(() => {
    apiMocks.getHealth.mockReset();
  });

  it("renders combined THOR and MAYA engine health cards", async () => {
    apiMocks.getHealth.mockResolvedValue({
      ok: true,
      time: "2026-03-11T12:00:00Z",
      build: {
        version: "test",
        commit: "abc123",
        build_time: "2026-03-11T11:00:00Z",
      },
      liquidity_engines: {
        THOR: {
          protocol: "THOR",
          thornode_sources: ["https://thornode.example"],
          midgard_sources: ["https://midgard.example"],
          legacy_action_sources: ["https://legacy.example"],
        },
        MAYA: {
          protocol: "MAYA",
          thornode_sources: ["https://mayanode.example"],
          midgard_sources: ["https://maya-midgard.example"],
          legacy_action_sources: [],
        },
      },
      tracker_providers: {},
      tracker_overrides: {},
      tracker_candidates: {},
      tracker_health: {},
      tracker_sources: {},
    });

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <HealthPanel />
      </QueryClientProvider>
    );

    await screen.findByText("Liquidity Engines");
    expect(screen.getByText("THOR")).toBeTruthy();
    expect(screen.getByText("MAYA")).toBeTruthy();
    expect(screen.getByText("THORNode 1 • Midgard 1 • Legacy 1")).toBeTruthy();
    expect(screen.getByText("MAYANode 1 • Midgard 1")).toBeTruthy();
  });

  it("tolerates null legacy action sources from the health API", async () => {
    apiMocks.getHealth.mockResolvedValue({
      ok: true,
      time: "2026-03-11T12:00:00Z",
      build: {
        version: "test",
        commit: "abc123",
        build_time: "2026-03-11T11:00:00Z",
      },
      liquidity_engines: {
        MAYA: {
          protocol: "MAYA",
          thornode_sources: ["https://mayanode.example"],
          midgard_sources: ["https://maya-midgard.example"],
          legacy_action_sources: null,
        },
      },
      tracker_providers: {},
      tracker_overrides: {},
      tracker_candidates: {},
      tracker_health: {},
      tracker_sources: {},
    });

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <HealthPanel />
      </QueryClientProvider>
    );

    await screen.findByText("MAYA");
    expect(screen.getByText("MAYANode 1 • Midgard 1")).toBeTruthy();
  });
});
