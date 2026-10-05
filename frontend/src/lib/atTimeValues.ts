import type { FlowAssetValue, FlowEdge, FlowEdgeTransaction, SupportingAction } from "./types";

type Valued = { usd_spot: number; usd_at_time?: number; usd_now?: number };

// preferValue makes usd_spot carry the value at transaction time when the
// server provided one, keeping today's value in usd_now. Graph code reads
// usd_spot everywhere, so normalizing once at the API boundary values every
// label, weight, and filter at transaction time. Graphs saved before
// usd_at_time existed pass through unchanged.
function preferValue<T extends Valued>(value: T): T {
  if (typeof value.usd_at_time !== "number" || typeof value.usd_now === "number") {
    return value;
  }
  return { ...value, usd_now: value.usd_spot, usd_spot: value.usd_at_time };
}

function preferAssets(assets: FlowAssetValue[] | undefined) {
  return (assets ?? []).map((asset) => preferValue(asset));
}

export function preferAtTimeValues<T extends { edges?: FlowEdge[]; supporting_actions?: SupportingAction[] }>(graph: T): T {
  return {
    ...graph,
    edges: graph.edges?.map((edge) => ({
      ...preferValue(edge),
      assets: preferAssets(edge.assets),
      transactions: (edge.transactions ?? []).map((tx: FlowEdgeTransaction) => ({
        ...preferValue(tx),
        assets: preferAssets(tx.assets),
      })),
    })),
    supporting_actions: graph.supporting_actions?.map((action) => preferValue(action)),
  };
}
