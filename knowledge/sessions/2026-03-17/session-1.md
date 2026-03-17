# Session 1 - Mouse + Trackpad Coexistence in Graph Canvas

> Date: 2026-03-17
> Focus: Improving wheel event heuristics so mouse scroll-wheel and MacBook trackpad work simultaneously on the graph canvas

## Summary

Improved the `shouldPanFromWheel` heuristic in the graph canvas so that a physical mouse and a MacBook trackpad can coexist without one breaking the other. Added Chrome's `wheelDelta` multiple-of-120 detection as the primary discriminator, extended the trackpad gesture lock from 180ms to 400ms to cover inertia scrolling, and bumped the fallback pixel-delta threshold from 96 to 100.

## Work Done

- Analyzed the existing `shouldPanFromWheel` logic and identified three weak spots: missing `wheelDelta` heuristic, gesture lock too short for inertia, pixel threshold too low for fast swipes
- Added `isMouseWheelByWheelDelta()` — uses Chrome/Edge's non-standard `wheelDelta` property (multiples of 120 = mouse wheel) as the most reliable mouse-vs-trackpad discriminator
- Extended `TRACKPAD_PAN_GESTURE_LOCK_MS` from 180 → 400 to cover trackpad inertia scrolling
- Bumped `TRACKPAD_PAN_PIXEL_DELTA_THRESHOLD` from 96 → 100 to align with standard macOS mouse wheel quantum
- Restructured `shouldPanFromWheel` into a clearer decision cascade with comments explaining each check
- Added 3 new test cases: wheelDelta mouse detection, wheelDelta trackpad pass-through, inertia continuation beyond 200ms

## Discoveries

- Chrome/Edge expose `WheelEvent.wheelDelta` (non-standard, deprecated but still present). Physical mouse wheels always produce exact multiples of 120; trackpad gestures produce other values. This is the single most reliable discriminator for ~75% of desktop web users.
- Trackpad inertia scrolling on macOS fires wheel events for 300-500ms+ after finger lift — a 180ms gesture lock window causes a jarring pan→zoom switch mid-gesture.
- The Figma/design-tool model (wheel always zooms, Space+drag to pan) sidesteps device detection entirely but changes the interaction model.

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/features/shared/graph-canvas/useGraphCanvasInteractions.ts` | Added `isMouseWheelByWheelDelta()`, rewrote `shouldPanFromWheel` cascade, tuned constants |
| `frontend/src/features/shared/graph-canvas/__tests__/GraphCanvas.multiSelectContextMenu.test.tsx` | Added 3 new test cases for wheelDelta detection and inertia lock |

## In Progress

None - session complete.

## Next Steps

- [ ] Manual QA with a physical mouse + MacBook trackpad to validate real-world feel
- [ ] Consider Option B (Figma-style wheel-always-zooms) as a future fallback if edge cases persist with smooth-scroll mice on Firefox/Safari
- [ ] Test with Logitech MX Master smooth-scroll mode specifically (the worst-case input device)
