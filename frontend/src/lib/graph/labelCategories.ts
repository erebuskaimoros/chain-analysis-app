import type { FlowNode } from "../types";
import type { VisibleGraph } from "./types";

export function nodeLabelCategory(node: Pick<FlowNode, "metrics">): string {
  const category = node.metrics?.label_category;
  return typeof category === "string" ? category : "";
}

// labelCategoriesOf lists the label categories present on a graph's nodes.
export function labelCategoriesOf(nodes: Array<Pick<FlowNode, "metrics">> | undefined): string[] {
  const categories = new Set<string>();
  for (const node of nodes ?? []) {
    const category = nodeLabelCategory(node);
    if (category) {
      categories.add(category);
    }
  }
  return Array.from(categories).sort();
}

// applyLabelCategoryFilter hides nodes whose label category is hidden, along
// with every edge that touches them.
export function applyLabelCategoryFilter(graph: VisibleGraph | null, hidden: string[] | undefined): VisibleGraph | null {
  if (!graph || !hidden?.length) {
    return graph;
  }
  const hiddenSet = new Set(hidden);
  const nodes = graph.nodes.filter((node) => !hiddenSet.has(nodeLabelCategory(node)));
  const kept = new Set(nodes.map((node) => node.id));
  return {
    ...graph,
    nodes,
    edges: graph.edges.filter((edge) => kept.has(edge.source) && kept.has(edge.target)),
  };
}
