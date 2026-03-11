import cytoscape from "cytoscape";
import ELK from "elkjs/lib/elk.bundled.js";
import { graphLayoutNodeSize, type VisibleGraphNode } from "../../../lib/graph";

export async function applyElkLayout(
  cy: cytoscape.Core,
  mode: "actor" | "explorer",
  nodes: VisibleGraphNode[],
  elk: InstanceType<typeof ELK>
) {
  const graph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.edgeRouting": "POLYLINE",
      "elk.layered.spacing.nodeNodeBetweenLayers": "110",
      "elk.spacing.nodeNode": "42",
      "elk.padding": "[top=32,left=32,bottom=32,right=32]",
    },
    children: nodes.map((node) => {
      const size = graphLayoutNodeSize(mode, node);
      return {
        id: node.id,
        width: size.width,
        height: size.height,
      };
    }),
    edges: cy
      .edges()
      .toArray()
      .map((edge) => ({
        id: edge.id(),
        sources: [String(edge.data("source"))],
        targets: [String(edge.data("target"))],
      })),
  };

  const result = await elk.layout(graph);
  const positions = new Map(
    (result.children || []).map((child: { id: string; x?: number; y?: number }) => [
      child.id,
      { x: child.x || 0, y: child.y || 0 },
    ])
  );

  const positionMap: Record<string, { x: number; y: number }> = {};
  nodes.forEach((node) => {
    positionMap[node.id] =
      positions.get(node.id) || { x: Number(node.depth || 0) * 180, y: 80 };
  });

  cy.layout({
    name: "preset",
    fit: false,
    animate: false,
    positions: positionMap,
  }).run();
}
