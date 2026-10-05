import { describe, expect, it } from "vitest";
import { applyLabelCategoryFilter, labelCategoriesOf } from "../labelCategories";
import type { VisibleGraph } from "../types";

describe("label category filter", () => {
  const graph = {
    nodes: [
      { id: "a", metrics: { label_category: "exchange" } },
      { id: "b", metrics: {} },
      { id: "c", metrics: { label_category: "sanctioned" } },
    ],
    edges: [
      { id: "ab", source: "a", target: "b" },
      { id: "bc", source: "b", target: "c" },
    ],
  } as unknown as VisibleGraph;

  it("lists categories present on nodes", () => {
    expect(labelCategoriesOf(graph.nodes)).toEqual(["exchange", "sanctioned"]);
  });

  it("hides nodes in hidden categories and their edges", () => {
    const filtered = applyLabelCategoryFilter(graph, ["exchange"]);
    expect(filtered?.nodes.map((node) => node.id)).toEqual(["b", "c"]);
    expect(filtered?.edges.map((edge) => edge.id)).toEqual(["bc"]);
    expect(applyLabelCategoryFilter(graph, [])).toBe(graph);
  });
});
