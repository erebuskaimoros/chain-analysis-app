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
    const exists =
      selection.kind === "node"
        ? visibleGraph.nodes.some((node) => node.id === selection.node.id)
        : visibleGraph.edges.some((edge) => edge.id === selection.edge.id);
    if (!exists) {
      setSelection(null);
    }
  }, [selection, setSelection, visibleGraph]);
}
