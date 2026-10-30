import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { buildHash, parseHash, RouterProvider, useRouteIntent, useRouter } from "../router";

describe("hash routes", () => {
  it("reads the view and its parameters", () => {
    expect(parseHash("#explorer?address=thor1abc")).toEqual({ view: "explorer", params: { address: "thor1abc" } });
    expect(parseHash("#graph")).toEqual({ view: "graph", params: {} });
  });

  it("keeps old links working and falls back to Home", () => {
    expect(parseHash("#overview").view).toBe("home");
    expect(parseHash("#nowhere").view).toBe("home");
    expect(parseHash("").view).toBe("home");
  });

  it("writes parameters into the hash and drops empty ones", () => {
    expect(buildHash("trace", { seed: "ETH|0xabc", trace: "" })).toBe("#trace?seed=ETH%7C0xabc");
    expect(buildHash("cases")).toBe("#cases");
  });
});

function IntentProbe({ onIntent }: { onIntent: (params: Record<string, string>) => void }) {
  const { navigate } = useRouter();
  useRouteIntent("trace", onIntent);
  return (
    <button type="button" onClick={() => navigate("trace", { seed: "thor1abc" })}>
      Trace
    </button>
  );
}

describe("useRouteIntent", () => {
  afterEach(() => {
    cleanup();
    window.location.hash = "";
  });

  it("hands each link's parameters to the page once and clears them from the address bar", () => {
    const onIntent = vi.fn();
    render(
      <RouterProvider>
        <IntentProbe onIntent={onIntent} />
      </RouterProvider>
    );
    expect(onIntent).not.toHaveBeenCalled();

    act(() => {
      screen.getByRole("button", { name: "Trace" }).click();
    });

    expect(onIntent).toHaveBeenCalledTimes(1);
    expect(onIntent).toHaveBeenCalledWith({ seed: "thor1abc" });
    expect(window.location.hash).toBe("#trace");
  });
});
