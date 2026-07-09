import { createRef } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GraphCanvasOverlays } from "../GraphCanvasOverlays";

afterEach(cleanup);

function baseProps() {
  return {
    filterPopoverRef: createRef<HTMLDivElement>(),
    menuRef: createRef<HTMLDivElement>(),
    menuState: null,
    doubleActivateLabel: "Expand one hop",
    showSaveState: true,
    search: {
      query: "",
      setQuery: vi.fn(),
      matches: [],
      activeIndex: 0,
      next: vi.fn(),
      prev: vi.fn(),
      clear: vi.fn(),
    },
    searchInputRef: createRef<HTMLInputElement>(),
    wheelMode: "auto" as const,
    onCycleWheelMode: vi.fn(),
    hoverCard: null,
    minimapCanvasRef: createRef<HTMLCanvasElement>(),
    onToolbarAction: vi.fn(),
    onContextMenuAction: vi.fn(),
  };
}

describe("GraphCanvasOverlays", () => {
  it("shows a save-state button and dispatches the save toolbar action", () => {
    const props = baseProps();

    render(<GraphCanvasOverlays {...props} />);

    fireEvent.click(screen.getByTitle("Save graph state"));

    expect(props.onToolbarAction).toHaveBeenCalledWith("save");
  });

  it("cycles the scroll wheel mode preference", () => {
    const props = baseProps();

    render(<GraphCanvasOverlays {...props} />);

    fireEvent.click(screen.getByRole("button", { name: "Auto" }));

    expect(props.onCycleWheelMode).toHaveBeenCalled();
  });

  it("steps through search matches with Enter and shift+Enter", () => {
    const props = baseProps();
    props.search.query = "abc";

    render(<GraphCanvasOverlays {...props} />);

    const input = screen.getByPlaceholderText("Search nodes (/)");
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(props.search.next).toHaveBeenCalledTimes(1);
    expect(props.search.prev).toHaveBeenCalledTimes(1);
    expect(props.search.clear).toHaveBeenCalledTimes(1);
  });
});
