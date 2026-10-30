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
    savePrompt: null as { defaultName: string } | null,
    onSaveConfirm: vi.fn(),
    onSaveCancel: vi.fn(),
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

    const input = screen.getByPlaceholderText("Find a node (/)");
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(props.search.next).toHaveBeenCalledTimes(1);
    expect(props.search.prev).toHaveBeenCalledTimes(1);
    expect(props.search.clear).toHaveBeenCalledTimes(1);
  });

  it("confirms or cancels the save-name popover", () => {
    const props = baseProps();
    props.savePrompt = { defaultName: "Case X" };

    render(<GraphCanvasOverlays {...props} />);

    const input = screen.getByLabelText("Save graph state");
    expect((input as HTMLInputElement).value).toBe("Case X");

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(props.onSaveConfirm).toHaveBeenCalledWith("Case X");

    fireEvent.keyDown(input, { key: "Escape" });
    expect(props.onSaveCancel).toHaveBeenCalled();
  });
});
