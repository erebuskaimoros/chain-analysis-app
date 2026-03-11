import { createRef } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { GraphCanvasOverlays } from "../GraphCanvasOverlays";

describe("GraphCanvasOverlays", () => {
  it("shows a save-state button and dispatches the save toolbar action", () => {
    const onToolbarAction = vi.fn();

    render(
      <GraphCanvasOverlays
        filterPopoverRef={createRef()}
        menuRef={createRef()}
        menuState={null}
        doubleActivateLabel="Expand one hop"
        showSaveState
        onToolbarAction={onToolbarAction}
        onContextMenuAction={() => undefined}
      />
    );

    fireEvent.click(screen.getByTitle("Save graph state"));

    expect(onToolbarAction).toHaveBeenCalledWith("save");
  });
});
