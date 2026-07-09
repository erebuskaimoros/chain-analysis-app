import { useEffect, useRef, type MutableRefObject } from "react";
import cytoscape from "cytoscape";
import { graphStylesheet, type GraphSelection, type VisibleGraphEdge, type VisibleGraphNode } from "../../../lib/graph";
import type { SavedGraphCanvasState } from "../../../lib/graphState";
import { applyElkLayout, placeNewNodesNearAnchors } from "./layout";
import { selectedGraphNodes } from "./utils";

const DOUBLE_TAP_WINDOW_MS = 320;
export const GRAPH_MIN_ZOOM = 0.05;
export const GRAPH_MAX_ZOOM = 10;

interface UseGraphCanvasCoreOptions {
  mode: "actor" | "explorer";
  nodes: VisibleGraphNode[];
  edges: VisibleGraphEdge[];
  selection: GraphSelection;
  onSelectionChange: (selection: GraphSelection) => void;
  onNodePrimaryAction?: (node: VisibleGraphNode) => boolean;
  nodeHasPrimaryAction?: (node: VisibleGraphNode) => boolean;
  onNodeDoubleActivate?: (node: VisibleGraphNode) => void;
  graphResetKey: number;
  savedCanvasState?: SavedGraphCanvasState | null;
  cyRef: MutableRefObject<cytoscape.Core | null>;
  viewportRef: MutableRefObject<{ zoom: number; pan: cytoscape.Position } | null>;
  suppressTapUntilRef: MutableRefObject<number>;
  surfaceRef: MutableRefObject<HTMLDivElement | null>;
  cyMountRef: MutableRefObject<HTMLDivElement | null>;
  scheduleLabelRender: () => void;
  cancelScheduledLabelRender: () => void;
}

export function useGraphCanvasCore({
  mode,
  nodes,
  edges,
  selection,
  onSelectionChange,
  onNodePrimaryAction,
  nodeHasPrimaryAction,
  onNodeDoubleActivate,
  graphResetKey,
  savedCanvasState,
  cyRef,
  viewportRef,
  suppressTapUntilRef,
  surfaceRef,
  cyMountRef,
  scheduleLabelRender,
  cancelScheduledLabelRender,
}: UseGraphCanvasCoreOptions) {
  const layoutSeqRef = useRef(0);
  const lastTapRef = useRef<{ id: string; at: number }>({ id: "", at: 0 });
  const nodeTapTimerRef = useRef<number | null>(null);
  const selectionRef = useRef(selection);
  const selectionChangeRef = useRef(onSelectionChange);
  const nodePrimaryActionRef = useRef(onNodePrimaryAction);
  const nodeHasPrimaryActionRef = useRef(nodeHasPrimaryAction);
  const nodeDoubleActivateRef = useRef(onNodeDoubleActivate);
  const nodeMapRef = useRef(new Map<string, VisibleGraphNode>());
  const edgeMapRef = useRef(new Map<string, VisibleGraphEdge>());
  const resetKeyRef = useRef(graphResetKey);
  const restoredCanvasStateKeyRef = useRef<number | null>(null);
  const topologySignatureRef = useRef("");

  selectionRef.current = selection;
  selectionChangeRef.current = onSelectionChange;
  nodePrimaryActionRef.current = onNodePrimaryAction;
  nodeHasPrimaryActionRef.current = nodeHasPrimaryAction;
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
      minZoom: GRAPH_MIN_ZOOM,
      maxZoom: GRAPH_MAX_ZOOM,
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

      const applyTap = () => {
        if (nodePrimaryActionRef.current?.(tapped)) {
          return;
        }
        const selectedNodes = selectedGraphNodes(cy);
        if (selectedNodes.length > 1) {
          selectionChangeRef.current({ kind: "nodes", nodes: selectedNodes });
          return;
        }
        selectionChangeRef.current({ kind: "node", node: tapped });
      };

      // Only defer when the tap may trigger a primary action, which mutates the
      // view and must not fire on the first tap of a double-tap. Plain nodes
      // select immediately; a following double-tap still expands.
      const mustDefer = nodePrimaryActionRef.current
        ? nodeHasPrimaryActionRef.current
          ? nodeHasPrimaryActionRef.current(tapped)
          : true
        : false;
      if (mustDefer) {
        nodeTapTimerRef.current = window.setTimeout(() => {
          nodeTapTimerRef.current = null;
          lastTapRef.current = { id: "", at: 0 };
          applyTap();
        }, DOUBLE_TAP_WINDOW_MS);
      } else {
        applyTap();
      }
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

    cy.on("zoom pan", () => {
      viewportRef.current = { zoom: cy.zoom(), pan: cy.pan() };
    });
    cy.on("render zoom pan resize add remove data position", () => {
      scheduleLabelRender();
    });

    cyRef.current = cy;
    topologySignatureRef.current = "";

    return () => {
      clearPendingNodeTap();
      cancelScheduledLabelRender();
      cy.destroy();
      cyRef.current = null;
      topologySignatureRef.current = "";
    };
  }, [cancelScheduledLabelRender, cyMountRef, mode, scheduleLabelRender, surfaceRef]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) {
      return;
    }

    const shouldRestoreSavedCanvasState =
      Boolean(savedCanvasState) && restoredCanvasStateKeyRef.current !== graphResetKey;
    const resetLayout = resetKeyRef.current !== graphResetKey || shouldRestoreSavedCanvasState;
    const nextTopologySignature = graphTopologySignature(nodes, edges);
    const topologyChanged = topologySignatureRef.current !== nextTopologySignature;
    const canvasIsEmpty = cy.elements().length === 0 && (nodes.length > 0 || edges.length > 0);

    if (resetKeyRef.current !== graphResetKey) {
      resetKeyRef.current = graphResetKey;
      viewportRef.current = null;
    }
    if (shouldRestoreSavedCanvasState) {
      restoredCanvasStateKeyRef.current = graphResetKey;
      viewportRef.current = savedCanvasState?.viewport ? { ...savedCanvasState.viewport } : null;
    }

    if (resetLayout || canvasIsEmpty) {
      // Fresh graph (or explicit reset / saved-state restore): rebuild and run a
      // full ELK layout, seeding saved positions when restoring.
      const preservedPositions = shouldRestoreSavedCanvasState
        ? mapSavedCanvasNodePositions(savedCanvasState)
        : new Map<string, cytoscape.Position>();

      cy.batch(() => {
        cy.elements().remove();
        cy.add(buildElementDefinitions(nodes, edges));
      });
      topologySignatureRef.current = nextTopologySignature;

      const currentLayoutSeq = ++layoutSeqRef.current;
      const isStale = () => layoutSeqRef.current !== currentLayoutSeq || !cyRef.current;
      void applyElkLayout(cy, mode, nodes, preservedPositions, isStale).then(() => {
        if (isStale()) {
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
    } else if (topologyChanged) {
      // Incremental change (expansion, filter toggle): diff elements in place and
      // anchor new nodes next to their already-positioned neighbors. Existing
      // positions and the viewport stay untouched so the user's mental map and
      // manual arrangement survive.
      // Invalidate any still-pending full layout so it can't stomp the
      // positions applied here after its async work resolves.
      layoutSeqRef.current += 1;
      const newNodeIDs = diffGraphElements(cy, nodes, edges);
      topologySignatureRef.current = nextTopologySignature;
      if (newNodeIDs.length) {
        placeNewNodesNearAnchors(cy, mode, nodes, edges, newNodeIDs);
      }
      scheduleLabelRender();
    } else {
      syncElementData(cy, nodes, edges);
      scheduleLabelRender();
    }

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
  }, [edges, graphResetKey, mode, nodes, savedCanvasState, scheduleLabelRender]);

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

function mapSavedCanvasNodePositions(savedCanvasState: SavedGraphCanvasState | null | undefined) {
  const positions = new Map<string, cytoscape.Position>();
  if (!savedCanvasState) {
    return positions;
  }
  Object.entries(savedCanvasState.node_positions).forEach(([id, position]) => {
    positions.set(id, { x: position.x, y: position.y });
  });
  return positions;
}

function nodeElementData(node: VisibleGraphNode) {
  return {
    ...node,
    id: node.id,
    kind: node.kind,
    depth: node.depth,
  };
}

function edgeElementData(edge: VisibleGraphEdge) {
  return {
    ...edge,
    id: edge.id,
    source: edge.source,
    target: edge.target,
    actionClass: edge.action_class,
    edgeLabel: edge.edgeLabel,
    lineColor: edge.lineColor,
    width: edge.width,
  };
}

function buildElementDefinitions(nodes: VisibleGraphNode[], edges: VisibleGraphEdge[]): cytoscape.ElementDefinition[] {
  return [
    ...nodes.map((node) => ({ data: nodeElementData(node) })),
    ...edges.map((edge) => ({ data: edgeElementData(edge) })),
  ];
}

// Adds new elements, removes departed ones, and refreshes data on survivors.
// Returns the IDs of nodes that did not exist before the sync.
function diffGraphElements(cy: cytoscape.Core, nodes: VisibleGraphNode[], edges: VisibleGraphEdge[]) {
  const nodeIDs = new Set(nodes.map((node) => node.id));
  const edgeIDs = new Set(edges.map((edge) => edge.id));
  const newNodes: VisibleGraphNode[] = [];
  const newEdges: VisibleGraphEdge[] = [];

  cy.batch(() => {
    cy.edges().forEach((element) => {
      if (!edgeIDs.has(element.id())) {
        element.remove();
      }
    });
    cy.nodes().forEach((element) => {
      if (!nodeIDs.has(element.id())) {
        element.remove();
      }
    });
    nodes.forEach((node) => {
      const element = cy.getElementById(node.id);
      if (element.nonempty()) {
        element.data(nodeElementData(node));
      } else {
        newNodes.push(node);
      }
    });
    edges.forEach((edge) => {
      const element = cy.getElementById(edge.id);
      if (element.nonempty()) {
        element.data(edgeElementData(edge));
      } else {
        newEdges.push(edge);
      }
    });
    if (newNodes.length || newEdges.length) {
      cy.add(buildElementDefinitions(newNodes, newEdges));
    }
  });

  return newNodes.map((node) => node.id);
}

function syncElementData(cy: cytoscape.Core, nodes: VisibleGraphNode[], edges: VisibleGraphEdge[]) {
  const nodeDataByID = new Map(nodes.map((node) => [node.id, nodeElementData(node)]));
  const edgeDataByID = new Map(edges.map((edge) => [edge.id, edgeElementData(edge)]));

  cy.batch(() => {
    cy.nodes().forEach((element) => {
      const nextData = nodeDataByID.get(element.id());
      if (nextData) {
        element.data(nextData);
      }
    });
    cy.edges().forEach((element) => {
      const nextData = edgeDataByID.get(element.id());
      if (nextData) {
        element.data(nextData);
      }
    });
  });
}

function graphTopologySignature(nodes: VisibleGraphNode[], edges: VisibleGraphEdge[]) {
  const nodeSignature = nodes
    .map((node) => `${node.id}|${node.kind}|${node.stage}|${node.depth}`)
    .sort()
    .join("~");
  const edgeSignature = edges
    .map((edge) => `${edge.id}|${edge.source}|${edge.target}`)
    .sort()
    .join("~");
  return `${nodeSignature}::${edgeSignature}`;
}
