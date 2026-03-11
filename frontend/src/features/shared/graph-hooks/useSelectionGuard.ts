import { useEffect } from "react";
import type { Dispatch, SetStateAction } from "react";
import type { GraphSelection, VisibleGraph } from "../../../lib/graph";

export function useSelectionGuard(
  selection: GraphSelection,
  setSelection: Dispatch<SetStateAction<GraphSelection>>,
  visibleGraph: VisibleGraph | null
) {
  useEffect(() => {
    if (!visibleGraph || !selection) {
      return;
    }
    if (selection.kind === "node") {
      const nextNode = visibleGraph.nodes.find((node) => node.id === selection.node.id);
      if (!nextNode) {
        setSelection(null);
      }
      return;
    }
    if (selection.kind === "nodes") {
      const nextNodes = selection.nodes
        .map((selectedNode) => visibleGraph.nodes.find((node) => node.id === selectedNode.id) ?? null)
        .filter(Boolean) as VisibleGraph["nodes"];
      if (!nextNodes.length) {
        setSelection(null);
        return;
      }
      if (nextNodes.length === 1) {
        setSelection({ kind: "node", node: nextNodes[0] });
        return;
      }
      const unchanged =
        nextNodes.length === selection.nodes.length &&
        nextNodes.every((node, index) => node.id === selection.nodes[index]?.id);
      if (!unchanged) {
        setSelection({ kind: "nodes", nodes: nextNodes });
      }
      return;
    }
    const nextEdge = visibleGraph.edges.find((edge) => edge.id === selection.edge.id);
    if (!nextEdge) {
      setSelection(null);
    }
  }, [selection, setSelection, visibleGraph]);
}
