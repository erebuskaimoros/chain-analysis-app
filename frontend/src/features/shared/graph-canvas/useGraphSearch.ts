import { useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import type cytoscape from "cytoscape";
import type { VisibleGraphNode } from "../../../lib/graph";

export function graphSearchMatches(nodes: VisibleGraphNode[], query: string): VisibleGraphNode[] {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return [];
  }
  return nodes.filter((node) => searchableNodeText(node).includes(needle));
}

function searchableNodeText(node: VisibleGraphNode) {
  const address = node.metrics && typeof node.metrics === "object" ? String(node.metrics.address ?? "") : "";
  return [node.displayLabel, node.label, node.id, address, node.chain, node.kind]
    .map((value) => String(value ?? ""))
    .join(" ")
    .toLowerCase();
}

// Owns the search box state, marks matches on the canvas, and centers the
// active match. Returns everything the toolbar search UI needs.
export function useGraphSearch(
  cyRef: MutableRefObject<cytoscape.Core | null>,
  nodes: VisibleGraphNode[],
  scheduleLabelRender: () => void
) {
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const appliedRef = useRef(false);

  const matches = useMemo(() => graphSearchMatches(nodes, query), [nodes, query]);
  const clampedIndex = matches.length ? Math.min(activeIndex, matches.length - 1) : 0;

  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy || typeof cy.nodes !== "function") {
      return;
    }
    const allNodes = cy.nodes() as cytoscape.CollectionReturnValue;
    if (typeof allNodes.removeClass !== "function") {
      return;
    }

    if (appliedRef.current) {
      allNodes.removeClass("graph-search-match graph-search-active");
      appliedRef.current = false;
    }
    if (!matches.length) {
      scheduleLabelRender();
      return;
    }

    matches.forEach((match) => {
      const element = cy.getElementById(match.id);
      if (element.nonempty() && typeof element.addClass === "function") {
        element.addClass("graph-search-match");
      }
    });

    const active = matches[clampedIndex];
    const activeElement = cy.getElementById(active.id);
    if (activeElement.nonempty() && typeof activeElement.addClass === "function") {
      activeElement.addClass("graph-search-active");
      centerOnElement(cy, activeElement);
    }
    appliedRef.current = true;
    scheduleLabelRender();
  }, [clampedIndex, cyRef, matches, scheduleLabelRender]);

  function step(delta: number) {
    if (!matches.length) {
      return;
    }
    setActiveIndex((current) => {
      const bounded = matches.length ? ((current + delta) % matches.length + matches.length) % matches.length : 0;
      return bounded;
    });
  }

  function clear() {
    setQuery("");
    setActiveIndex(0);
  }

  return {
    query,
    setQuery,
    matches,
    activeIndex: clampedIndex,
    next: () => step(1),
    prev: () => step(-1),
    clear,
  };
}

function centerOnElement(cy: cytoscape.Core, element: cytoscape.CollectionReturnValue | cytoscape.SingularElementReturnValue) {
  const targetZoom = Math.max(Number(cy.zoom() || 1), 0.7);
  if (typeof cy.animate === "function") {
    cy.animate(
      {
        center: { eles: element as cytoscape.CollectionArgument },
        zoom: targetZoom,
      },
      { duration: 220, easing: "ease-in-out-quad" }
    );
    return;
  }
  if (typeof cy.center === "function") {
    cy.center(element as cytoscape.CollectionArgument);
  }
}
