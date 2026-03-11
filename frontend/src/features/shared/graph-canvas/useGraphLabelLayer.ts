import { useCallback, useRef, type MutableRefObject } from "react";
import type cytoscape from "cytoscape";
import type { VisibleGraphNode } from "../../../lib/graph";
import { escapeHTML, renderedNodeHeight } from "./utils";

export function useGraphLabelLayer(
  cyRef: MutableRefObject<cytoscape.Core | null>,
  surfaceRef: MutableRefObject<HTMLDivElement | null>
) {
  const labelLayerRef = useRef<HTMLDivElement | null>(null);
  const labelFrameRef = useRef<number | null>(null);

  const renderLabels = useCallback(() => {
    const cy = cyRef.current;
    const surface = surfaceRef.current;
    const layer = labelLayerRef.current;
    if (!cy || !surface || !layer) {
      return;
    }

    const width = surface.clientWidth;
    const height = surface.clientHeight;
    const viewportPadding = 140;
    const zoom = Number(cy.zoom() || 1);
    const labelScale = Math.max(0.3, Math.min(1.35, zoom));
    const labelFontPx = 11.84 * labelScale;
    const liveFontPx = 10.88 * labelScale;
    const labelMaxWidthPx = Math.max(48, Math.min(220, 150 * labelScale));
    const labelGapPx = Math.max(2, 8 * labelScale);
    const html: string[] = [];

    cy.nodes().forEach((node) => {
      const data = node.data() as VisibleGraphNode;
      const displayLabel = String(data.displayLabel || "").trim();
      const liveHoldingsLabel = String(data.live_holdings_label || "").trim();
      if (!displayLabel && !liveHoldingsLabel) {
        return;
      }

      const renderedPosition = node.renderedPosition();
      if (
        renderedPosition.x < -viewportPadding ||
        renderedPosition.x > width + viewportPadding ||
        renderedPosition.y < -viewportPadding ||
        renderedPosition.y > height + viewportPadding
      ) {
        return;
      }

      const heightPx = renderedNodeHeight(node);
      const topY = renderedPosition.y - heightPx / 2 - labelGapPx;
      const bottomY = renderedPosition.y + heightPx / 2 + labelGapPx;

      if (displayLabel) {
        html.push(
          `<div class="graph-node-text graph-node-label" style="left:${renderedPosition.x.toFixed(
            1
          )}px;top:${topY.toFixed(1)}px;font-size:${labelFontPx.toFixed(
            2
          )}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(displayLabel)}</div>`
        );
      }
      if (liveHoldingsLabel) {
        const unavailableClass = data.live_holdings_status === "error" ? " is-unavailable" : "";
        html.push(
          `<div class="graph-node-text graph-node-live${unavailableClass}" style="left:${renderedPosition.x.toFixed(
            1
          )}px;top:${bottomY.toFixed(1)}px;font-size:${liveFontPx.toFixed(
            2
          )}px;max-width:${labelMaxWidthPx.toFixed(1)}px;">${escapeHTML(liveHoldingsLabel)}</div>`
        );
      }
    });

    layer.innerHTML = html.join("");
  }, [cyRef, surfaceRef]);

  const cancelScheduledLabelRender = useCallback(() => {
    if (labelFrameRef.current !== null) {
      window.cancelAnimationFrame(labelFrameRef.current);
      labelFrameRef.current = null;
    }
  }, []);

  const scheduleLabelRender = useCallback(() => {
    if (!cyRef.current || !labelLayerRef.current || labelFrameRef.current !== null) {
      return;
    }
    labelFrameRef.current = window.requestAnimationFrame(() => {
      labelFrameRef.current = null;
      renderLabels();
    });
  }, [cyRef, renderLabels]);

  return {
    labelLayerRef,
    scheduleLabelRender,
    cancelScheduledLabelRender,
  };
}
