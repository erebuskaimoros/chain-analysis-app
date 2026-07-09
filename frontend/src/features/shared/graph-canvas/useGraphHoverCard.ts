import { useEffect, useRef, useState, type MutableRefObject } from "react";
import type cytoscape from "cytoscape";
import type { VisibleGraphNode } from "../../../lib/graph";
import type { GraphHoverCardState } from "./types";

const HOVER_SHOW_DELAY_MS = 180;

// Shows a hover card next to the node under the pointer, and switches the
// cursor to a pointer over nodes. Hides on any viewport change or interaction.
export function useGraphHoverCard(
  cyRef: MutableRefObject<cytoscape.Core | null>,
  surfaceRef: MutableRefObject<HTMLDivElement | null>
) {
  const [hoverCard, setHoverCard] = useState<GraphHoverCardState | null>(null);
  const showTimerRef = useRef<number | null>(null);

  useEffect(() => {
    const cy = cyRef.current;
    const surface = surfaceRef.current;
    if (!cy || !surface || typeof cy.on !== "function") {
      return;
    }
    const surfaceElement = surface;

    function clearShowTimer() {
      if (showTimerRef.current !== null) {
        window.clearTimeout(showTimerRef.current);
        showTimerRef.current = null;
      }
    }

    function hide() {
      clearShowTimer();
      setHoverCard(null);
    }

    function onNodeOver(event: cytoscape.EventObject) {
      const target = event.target as cytoscape.NodeSingular;
      if (typeof target.data !== "function" || typeof target.renderedPosition !== "function") {
        return;
      }
      surfaceElement.classList.add("is-node-hover");
      const node = target.data() as VisibleGraphNode;
      const renderedPosition = target.renderedPosition();
      clearShowTimer();
      showTimerRef.current = window.setTimeout(() => {
        showTimerRef.current = null;
        setHoverCard({ node, x: renderedPosition.x, y: renderedPosition.y });
      }, HOVER_SHOW_DELAY_MS);
    }

    function onNodeOut() {
      surfaceElement.classList.remove("is-node-hover");
      hide();
    }

    cy.on("mouseover", "node", onNodeOver);
    cy.on("mouseout", "node", onNodeOut);
    cy.on("grab", "node", hide);
    cy.on("pan zoom tapstart", hide);

    return () => {
      clearShowTimer();
      surfaceElement.classList.remove("is-node-hover");
      if (typeof cy.off === "function") {
        cy.off("mouseover", "node", onNodeOver);
        cy.off("mouseout", "node", onNodeOut);
        cy.off("grab", "node", hide);
        cy.off("pan zoom tapstart", hide);
      }
    };
  }, [cyRef, surfaceRef]);

  return hoverCard;
}
