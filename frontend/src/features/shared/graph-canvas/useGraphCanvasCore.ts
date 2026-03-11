import { useEffect, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import cytoscape from "cytoscape";
import ELK from "elkjs/lib/elk.bundled.js";
import { graphStylesheet, type GraphSelection, type VisibleGraphEdge, type VisibleGraphNode } from "../../../lib/graph";
import { applyElkLayout } from "./layout";
import { contextMenuPointFromGraphEvent } from "./utils";
import type { ContextMenuState } from "./types";

const DOUBLE_TAP_WINDOW_MS = 320;

interface UseGraphCanvasCoreOptions {
  mode: "actor" | "explorer";
  nodes: VisibleGraphNode[];
  edges: VisibleGraphEdge[];
  selection: GraphSelection;
  onSelectionChange: (selection: GraphSelection) => void;
  onNodePrimaryAction?: (node: VisibleGraphNode) => boolean;
  onNodeDoubleActivate?: (node: VisibleGraphNode) => void;
  graphResetKey: number;
  cyRef: MutableRefObject<cytoscape.Core | null>;
  viewportRef: MutableRefObject<{ zoom: number; pan: cytoscape.Position } | null>;
  suppressTapUntilRef: MutableRefObject<number>;
  surfaceRef: MutableRefObject<HTMLDivElement | null>;
  cyMountRef: MutableRefObject<HTMLDivElement | null>;
  scheduleLabelRender: () => void;
  cancelScheduledLabelRender: () => void;
  setMenuState: Dispatch<SetStateAction<ContextMenuState>>;
  lastPaneContextMenuOpenedAtRef: MutableRefObject<number>;
}

export function useGraphCanvasCore({
  mode,
  nodes,
  edges,
  selection,
  onSelectionChange,
  onNodePrimaryAction,
  onNodeDoubleActivate,
  graphResetKey,
  cyRef,
  viewportRef,
  suppressTapUntilRef,
  surfaceRef,
  cyMountRef,
  scheduleLabelRender,
  cancelScheduledLabelRender,
  setMenuState,
  lastPaneContextMenuOpenedAtRef,
}: UseGraphCanvasCoreOptions) {
  const elkRef = useRef(new ELK());
  const layoutSeqRef = useRef(0);
  const lastTapRef = useRef<{ id: string; at: number }>({ id: "", at: 0 });
  const nodeTapTimerRef = useRef<number | null>(null);
  const selectionRef = useRef(selection);
  const selectionChangeRef = useRef(onSelectionChange);
  const nodePrimaryActionRef = useRef(onNodePrimaryAction);
  const nodeDoubleActivateRef = useRef(onNodeDoubleActivate);
  const nodeMapRef = useRef(new Map<string, VisibleGraphNode>());
  const edgeMapRef = useRef(new Map<string, VisibleGraphEdge>());
  const resetKeyRef = useRef(graphResetKey);

  selectionRef.current = selection;
  selectionChangeRef.current = onSelectionChange;
  nodePrimaryActionRef.current = onNodePrimaryAction;
  nodeDoubleActivateRef.current = onNodeDoubleActivate;
  nodeMapRef.current = new Map(nodes.map((node) => [node.id, node]));
  edgeMapRef.current = new Map(edges.map((edge) => [edge.id, edge]));

  useEffect(() => {
    if (!cyMountRef.current) {
      return;
    }

    const cy = cytoscape({
      container: cyMountRef.current,
      elements: [],
      style: graphStylesheet(mode),
      wheelSensitivity: 0.3,
      zoomingEnabled: true,
      userZoomingEnabled: false,
      boxSelectionEnabled: false,
      selectionType: "additive",
      userPanningEnabled: false,
      autoungrabify: false,
    });

    function clearPendingNodeTap() {
      if (nodeTapTimerRef.current !== null) {
        window.clearTimeout(nodeTapTimerRef.current);
        nodeTapTimerRef.current = null;
      }
    }

    function graphTapSuppressed() {
      return Date.now() < suppressTapUntilRef.current;
    }

    cy.on("tap", "node", (event) => {
      if (graphTapSuppressed()) {
        return;
      }
      const tapped = nodeMapRef.current.get(event.target.id());
      if (!tapped) {
        return;
      }
      const now = Date.now();
      if (
        lastTapRef.current.id === tapped.id &&
        now - lastTapRef.current.at <= DOUBLE_TAP_WINDOW_MS &&
        nodeDoubleActivateRef.current
      ) {
        clearPendingNodeTap();
        lastTapRef.current = { id: "", at: 0 };
        nodeDoubleActivateRef.current(tapped);
        return;
      }

      lastTapRef.current = { id: tapped.id, at: now };
      clearPendingNodeTap();
      nodeTapTimerRef.current = window.setTimeout(() => {
        nodeTapTimerRef.current = null;
        lastTapRef.current = { id: "", at: 0 };
        if (nodePrimaryActionRef.current?.(tapped)) {
          return;
        }
        selectionChangeRef.current({ kind: "node", node: tapped });
      }, DOUBLE_TAP_WINDOW_MS);
    });

    cy.on("tap", "edge", (event) => {
      if (graphTapSuppressed()) {
        return;
      }
      const tapped = edgeMapRef.current.get(event.target.id());
      if (!tapped) {
        return;
      }
      selectionChangeRef.current({ kind: "edge", edge: tapped });
    });

    cy.on("tap", (event) => {
      if (event.target === cy && !graphTapSuppressed()) {
        selectionChangeRef.current(null);
      }
    });

    cy.on("cxttap", "node", (event) => {
      const node = nodeMapRef.current.get(event.target.id());
      const surface = surfaceRef.current;
      if (!node || !surface) {
        return;
      }
      const point = contextMenuPointFromGraphEvent(surface, event);
      const selection = selectionRef.current;
      if (
        selection?.kind === "nodes" &&
        selection.nodes.length > 1 &&
        selection.nodes.some((selectedNode) => selectedNode.id === node.id)
      ) {
        setMenuState({
          mode: "nodes",
          nodes: selection.nodes,
          x: point.x,
          y: point.y,
        });
        return;
      }
      setMenuState({
        mode: "node",
        node,
        x: point.x,
        y: point.y,
      });
    });

    cy.on("cxttap", (event) => {
      if (event.target !== cy || !surfaceRef.current) {
        return;
      }
      if (Date.now() - lastPaneContextMenuOpenedAtRef.current < 120) {
        return;
      }
      const point = contextMenuPointFromGraphEvent(surfaceRef.current, event);
      const selection = selectionRef.current;
      if (selection?.kind === "nodes" && selection.nodes.length > 1) {
        setMenuState({
          mode: "nodes",
          nodes: selection.nodes,
          x: point.x,
          y: point.y,
        });
        lastPaneContextMenuOpenedAtRef.current = Date.now();
        return;
      }
      setMenuState({
        mode: "pane",
        x: point.x,
        y: point.y,
      });
      lastPaneContextMenuOpenedAtRef.current = Date.now();
    });

    cy.on("zoom pan", () => {
      viewportRef.current = { zoom: cy.zoom(), pan: cy.pan() };
    });
    cy.on("render zoom pan resize add remove data position", () => {
      scheduleLabelRender();
    });

    cyRef.current = cy;

    return () => {
      clearPendingNodeTap();
      cancelScheduledLabelRender();
      cy.destroy();
      cyRef.current = null;
    };
  }, [cancelScheduledLabelRender, cyMountRef, lastPaneContextMenuOpenedAtRef, mode, scheduleLabelRender, setMenuState, surfaceRef]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) {
      return;
    }

    if (resetKeyRef.current !== graphResetKey) {
      resetKeyRef.current = graphResetKey;
      viewportRef.current = null;
    }

    const elements: cytoscape.ElementDefinition[] = [
      ...nodes.map((node) => ({
        data: {
          ...node,
          id: node.id,
          kind: node.kind,
          depth: node.depth,
        },
      })),
      ...edges.map((edge) => ({
        data: {
          ...edge,
          id: edge.id,
          source: edge.source,
          target: edge.target,
          actionClass: edge.action_class,
          edgeLabel: edge.edgeLabel,
          lineColor: edge.lineColor,
          width: edge.width,
        },
      })),
    ];

    cy.batch(() => {
      cy.elements().remove();
      cy.add(elements);
    });

    const currentLayoutSeq = ++layoutSeqRef.current;
    void applyElkLayout(cy, mode, nodes, elkRef.current).then(() => {
      if (layoutSeqRef.current !== currentLayoutSeq || !cyRef.current) {
        return;
      }
      const viewport = viewportRef.current;
      if (viewport) {
        cy.zoom(viewport.zoom);
        cy.pan(viewport.pan);
      } else {
        cy.fit(cy.elements(), 40);
      }
      scheduleLabelRender();
    });

    const selected = selectionRef.current;
    if (selected) {
      const exists =
        selected.kind === "node"
          ? nodeMapRef.current.has(selected.node.id)
          : selected.kind === "nodes"
          ? selected.nodes.some((node) => nodeMapRef.current.has(node.id))
          : edgeMapRef.current.has(selected.edge.id);
      if (!exists) {
        selectionChangeRef.current(null);
      }
    }
  }, [edges, graphResetKey, mode, nodes, scheduleLabelRender]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) {
      return;
    }
    cy.elements().unselect();
    if (!selection) {
      return;
    }
    if (selection.kind === "nodes") {
      selection.nodes.forEach((node) => {
        const element = cy.getElementById(node.id);
        if (element.nonempty()) {
          element.select();
        }
      });
      return;
    }
    const element = cy.getElementById(selection.kind === "node" ? selection.node.id : selection.edge.id);
    if (element.nonempty()) {
      element.select();
    }
  }, [selection]);

}
