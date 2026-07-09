import { useCallback, useEffect, useRef, type MutableRefObject } from "react";
import type cytoscape from "cytoscape";
import type { VisibleGraphNode } from "../../../lib/graph";
import { DEFAULT_GRAPH_LABEL_MAX_WIDTH_PX } from "./constants";
import { renderedNodeHeight } from "./utils";

// Below this zoom the text would be unreadable anyway; hide the whole layer
// instead of painting hundreds of tiny labels.
const LABEL_CULL_ZOOM = 0.32;

interface LabelEntry {
  root: HTMLDivElement;
  label: HTMLDivElement;
  live: HTMLDivElement;
  labelText: string;
  liveText: string;
  liveUnavailable: boolean;
}

// Renders node labels as a persistent pool of DOM elements keyed by node id.
// Per frame we only update transforms/typography of visible entries — no
// innerHTML rebuilds — which keeps panning smooth on large graphs.
export function useGraphLabelLayer(
  cyRef: MutableRefObject<cytoscape.Core | null>,
  surfaceRef: MutableRefObject<HTMLDivElement | null>
) {
  const labelLayerRef = useRef<HTMLDivElement | null>(null);
  const labelFrameRef = useRef<number | null>(null);
  const entriesRef = useRef(new Map<string, LabelEntry>());

  const renderLabels = useCallback(() => {
    const cy = cyRef.current;
    const surface = surfaceRef.current;
    const layer = labelLayerRef.current;
    if (!cy || !surface || !layer) {
      return;
    }

    const entries = entriesRef.current;
    const zoom = Number(cy.zoom() || 1);
    if (zoom < LABEL_CULL_ZOOM) {
      layer.style.display = "none";
      return;
    }
    layer.style.display = "";

    const width = surface.clientWidth;
    const height = surface.clientHeight;
    const viewportPadding = 140;
    const labelScale = Math.max(0.3, Math.min(1.35, zoom));
    const labelFontPx = (11.84 * labelScale).toFixed(2);
    const liveFontPx = (10.88 * labelScale).toFixed(2);
    const labelMaxWidthPx = Math.max(48, Math.min(220, DEFAULT_GRAPH_LABEL_MAX_WIDTH_PX * labelScale)).toFixed(1);
    const labelGapPx = Math.max(2, 8 * labelScale);
    const seen = new Set<string>();

    cy.nodes().forEach((node) => {
      const data = node.data() as VisibleGraphNode;
      const displayLabel = String(data.displayLabel || "").trim();
      const liveHoldingsLabel = String(data.live_holdings_label || "").trim();
      if (!displayLabel && !liveHoldingsLabel) {
        return;
      }

      const id = node.id();
      seen.add(id);
      let entry = entries.get(id);
      if (!entry) {
        entry = createLabelEntry(layer);
        entries.set(id, entry);
      }

      const renderedPosition = node.renderedPosition();
      const offscreen =
        renderedPosition.x < -viewportPadding ||
        renderedPosition.x > width + viewportPadding ||
        renderedPosition.y < -viewportPadding ||
        renderedPosition.y > height + viewportPadding;
      if (offscreen) {
        entry.root.style.display = "none";
        return;
      }
      entry.root.style.display = "";
      entry.root.style.transform = `translate3d(${renderedPosition.x.toFixed(1)}px, ${renderedPosition.y.toFixed(1)}px, 0)`;
      entry.root.style.opacity =
        typeof node.hasClass === "function" && node.hasClass("graph-dimmed") ? "0.15" : "";

      const halfHeight = renderedNodeHeight(node) / 2;
      if (displayLabel) {
        if (entry.labelText !== displayLabel) {
          entry.labelText = displayLabel;
          entry.label.textContent = displayLabel;
        }
        entry.label.style.display = "";
        entry.label.style.top = `${(-(halfHeight + labelGapPx)).toFixed(1)}px`;
        entry.label.style.fontSize = `${labelFontPx}px`;
        entry.label.style.maxWidth = `${labelMaxWidthPx}px`;
      } else {
        entry.label.style.display = "none";
      }

      if (liveHoldingsLabel) {
        if (entry.liveText !== liveHoldingsLabel) {
          entry.liveText = liveHoldingsLabel;
          entry.live.textContent = liveHoldingsLabel;
        }
        const unavailable = data.live_holdings_status === "error";
        if (entry.liveUnavailable !== unavailable) {
          entry.liveUnavailable = unavailable;
          entry.live.classList.toggle("is-unavailable", unavailable);
        }
        entry.live.style.display = "";
        entry.live.style.top = `${(halfHeight + labelGapPx).toFixed(1)}px`;
        entry.live.style.fontSize = `${liveFontPx}px`;
        entry.live.style.maxWidth = `${labelMaxWidthPx}px`;
      } else {
        entry.live.style.display = "none";
      }
    });

    entries.forEach((entry, id) => {
      if (!seen.has(id)) {
        entry.root.remove();
        entries.delete(id);
      }
    });
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

  useEffect(
    () => () => {
      entriesRef.current.forEach((entry) => entry.root.remove());
      entriesRef.current.clear();
    },
    []
  );

  return {
    labelLayerRef,
    scheduleLabelRender,
    cancelScheduledLabelRender,
  };
}

function createLabelEntry(layer: HTMLDivElement): LabelEntry {
  const root = document.createElement("div");
  root.className = "graph-node-anchor";
  const label = document.createElement("div");
  label.className = "graph-node-text graph-node-label";
  label.style.display = "none";
  const live = document.createElement("div");
  live.className = "graph-node-text graph-node-live";
  live.style.display = "none";
  root.appendChild(label);
  root.appendChild(live);
  layer.appendChild(root);
  return { root, label, live, labelText: "", liveText: "", liveUnavailable: false };
}
