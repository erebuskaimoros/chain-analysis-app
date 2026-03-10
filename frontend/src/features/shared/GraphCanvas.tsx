import { useEffect, useRef } from "react";
import cytoscape from "cytoscape";
import { edgeDisplayColor, edgeWidth, nodeDisplayColor, type GraphSelection } from "../../lib/graph";
import type { FlowEdge, FlowNode } from "../../lib/types";

interface GraphCanvasProps {
  nodes: FlowNode[];
  edges: FlowEdge[];
  selection: GraphSelection;
  onSelectionChange: (selection: GraphSelection) => void;
  onNodeDoubleActivate?: (node: FlowNode) => void;
}

export function GraphCanvas({ nodes, edges, selection, onSelectionChange, onNodeDoubleActivate }: GraphCanvasProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const cyRef = useRef<cytoscape.Core | null>(null);
  const lastTapRef = useRef<{ id: string; at: number }>({ id: "", at: 0 });
  const selectionChangeRef = useRef(onSelectionChange);
  const doubleActivateRef = useRef(onNodeDoubleActivate);

  useEffect(() => {
    selectionChangeRef.current = onSelectionChange;
  }, [onSelectionChange]);

  useEffect(() => {
    doubleActivateRef.current = onNodeDoubleActivate;
  }, [onNodeDoubleActivate]);

  useEffect(() => {
    if (!containerRef.current) {
      return;
    }

    const cy = cytoscape({
      container: containerRef.current,
      elements: [
        ...nodes.map((node) => ({
          data: {
            id: node.id,
            label: node.label,
            color: nodeDisplayColor(node),
            depth: node.depth,
            kind: node.kind,
          },
        })),
        ...edges.map((edge) => ({
          data: {
            id: edge.id,
            source: edge.from,
            target: edge.to,
            label: edge.action_label || edge.action_class,
            color: edgeDisplayColor(edge.action_class),
            width: edgeWidth(edge.usd_spot),
          },
        })),
      ],
      style: [
        {
          selector: "node",
          style: {
            "background-color": "data(color)",
            label: "data(label)",
            color: "#f8f5ef",
            "text-wrap": "wrap",
            "text-max-width": "140px",
            "text-valign": "center",
            "text-halign": "center",
            "font-size": 11,
            "border-color": "#ffffff",
            "border-width": 1.5,
            width: "label",
            height: "label",
            padding: "12px",
            shape: "round-rectangle",
          },
        },
        {
          selector: "edge",
          style: {
            label: "data(label)",
            "curve-style": "bezier",
            width: "data(width)",
            "line-color": "data(color)",
            "target-arrow-color": "data(color)",
            "target-arrow-shape": "triangle",
            "font-size": 9,
            color: "#31404c",
            "text-background-color": "#fff7ed",
            "text-background-opacity": 0.92,
            "text-background-padding": "3px",
            "text-rotation": "autorotate",
          },
        },
        {
          selector: ":selected",
          style: {
            "border-width": 3,
            "border-color": "#ffd166",
            "line-color": "#d46a24",
            "target-arrow-color": "#d46a24",
          },
        },
      ],
      layout: {
        name: "breadthfirst",
        fit: true,
        directed: true,
        padding: 36,
        spacingFactor: 1.3,
      },
    });

    cy.on("tap", "node", (event) => {
      const tapped = nodes.find((node) => node.id === event.target.id());
      if (!tapped) {
        return;
      }
      const now = Date.now();
      if (lastTapRef.current.id === tapped.id && now - lastTapRef.current.at < 320 && doubleActivateRef.current) {
        doubleActivateRef.current(tapped);
      }
      lastTapRef.current = { id: tapped.id, at: now };
      selectionChangeRef.current({ kind: "node", node: tapped });
    });

    cy.on("tap", "edge", (event) => {
      const tapped = edges.find((edge) => edge.id === event.target.id());
      if (!tapped) {
        return;
      }
      selectionChangeRef.current({ kind: "edge", edge: tapped });
    });

    cy.on("tap", (event) => {
      if (event.target === cy) {
        selectionChangeRef.current(null);
      }
    });

    cyRef.current = cy;
    return () => {
      cy.destroy();
      cyRef.current = null;
    };
  }, [edges, nodes]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) {
      return;
    }
    cy.elements().unselect();
    if (!selection) {
      return;
    }
    const element = cy.getElementById(selection.kind === "node" ? selection.node.id : selection.edge.id);
    if (element) {
      element.select();
    }
  }, [selection]);

  return <div className="graph-canvas" ref={containerRef} />;
}
