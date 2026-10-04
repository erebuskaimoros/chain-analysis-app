import { useEffect, useRef, type MutableRefObject } from "react";
import type cytoscape from "cytoscape";
import type { GraphSelection } from "../../../lib/graph";

// Dims everything except the selection and its closed neighborhood so incident
// flows pop out while tracing. Guards every collection call so the hook is a
// no-op against partial cytoscape doubles in tests.
export function useGraphNeighborhoodHighlight(
  cyRef: MutableRefObject<cytoscape.Core | null>,
  selection: GraphSelection,
  scheduleLabelRender: () => void
) {
  const appliedRef = useRef(false);

  useEffect(() => {
    const cy = cyRef.current;
    if (!cy || typeof cy.elements !== "function") {
      return;
    }
    const allElements = cy.elements() as cytoscape.CollectionReturnValue;
    if (typeof allElements.removeClass !== "function" || typeof allElements.addClass !== "function") {
      return;
    }

    if (appliedRef.current) {
      allElements.removeClass("graph-dimmed graph-highlighted");
      appliedRef.current = false;
    }

    const focus = collectSelectionElements(cy, selection);
    if (!focus || typeof focus.closedNeighborhood !== "function") {
      scheduleLabelRender();
      return;
    }

    const neighborhood = focus.union(focus.closedNeighborhood());
    allElements.addClass("graph-dimmed");
    neighborhood.removeClass("graph-dimmed");
    neighborhood.addClass("graph-highlighted");
    appliedRef.current = true;
    scheduleLabelRender();
  }, [cyRef, scheduleLabelRender, selection]);
}

function collectSelectionElements(cy: cytoscape.Core, selection: GraphSelection) {
  if (!selection) {
    return null;
  }
  const ids =
    selection.kind === "node"
      ? [selection.node.id]
      : selection.kind === "nodes"
      ? selection.nodes.map((node) => node.id)
      : [selection.edge.id];

  let collection: cytoscape.CollectionReturnValue | null = null;
  for (const id of ids) {
    const element = cy.getElementById(id);
    if (!element.nonempty() || typeof element.union !== "function") {
      continue;
    }
    collection = collection ? collection.union(element) : (element as cytoscape.CollectionReturnValue);
  }
  return collection;
}
