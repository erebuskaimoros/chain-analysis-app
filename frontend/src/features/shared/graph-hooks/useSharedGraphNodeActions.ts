import type { QueryClient } from "@tanstack/react-query";
import type { Dispatch, SetStateAction } from "react";
import {
  addToBlocklist,
  refreshLiveHoldings,
  upsertAnnotation,
} from "../../../lib/api";
import {
  applyNodeUpdates,
  explorerURLForAddress,
  nodeAddressForActions,
  rawNodesForVisibleNode,
  refreshableLiveValueNodes,
  unavailableRawNodes,
  type VisibleGraphNode,
} from "../../../lib/graph";
import type {
  ActorGraphResponse,
  AddressExplorerResponse,
  LiveHoldingsRefreshResponse,
} from "../../../lib/types";

interface UseSharedGraphNodeActionsOptions<TGraph extends ActorGraphResponse | AddressExplorerResponse> {
  graph: TGraph | null;
  setGraph: Dispatch<SetStateAction<TGraph | null>>;
  setStatusText: Dispatch<SetStateAction<string>>;
  queryClient: QueryClient;
  unavailableEmptyMessage: string;
  onRefreshNodeSuccess: (rawNodeCount: number, response: LiveHoldingsRefreshResponse) => string;
  onRefreshUnavailableSuccess: (requestedCount: number, response: LiveHoldingsRefreshResponse) => string;
}

export function useSharedGraphNodeActions<TGraph extends ActorGraphResponse | AddressExplorerResponse>({
  graph,
  setGraph,
  setStatusText,
  queryClient,
  unavailableEmptyMessage,
  onRefreshNodeSuccess,
  onRefreshUnavailableSuccess,
}: UseSharedGraphNodeActionsOptions<TGraph>) {
  function mergeRefreshResult(response: LiveHoldingsRefreshResponse) {
    setGraph((current) => {
      if (!current) {
        return current;
      }
      return {
        ...current,
        nodes: applyNodeUpdates(current.nodes, response.nodes),
        warnings: Array.from(new Set([...current.warnings, ...response.warnings])),
      };
    });
  }

  async function onOpenExplorer(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    const url = explorerURLForAddress(address, node.chain);
    if (!url) {
      setStatusText("Selected node does not resolve to a single explorer address.");
      return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
  }

  async function onCopyAddress(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    await navigator.clipboard.writeText(address);
    setStatusText(`Copied: ${address}`);
  }

  async function onRefreshLiveValue(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const rawNodes = rawNodesForVisibleNode(node, graph);
    if (!rawNodes.length) {
      setStatusText("Selected node has no live value context.");
      return;
    }
    const refreshableNodes = refreshableLiveValueNodes(rawNodes);
    if (!refreshableNodes.length) {
      setStatusText("Selected node live value is already computed inline.");
      return;
    }
    try {
      const response = await refreshLiveHoldings(refreshableNodes);
      mergeRefreshResult(response);
      setStatusText(onRefreshNodeSuccess(refreshableNodes.length, response));
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Live value refresh failed.");
    }
  }

  async function onRefreshUnavailable() {
    if (!graph) {
      return;
    }
    const rawNodes = refreshableLiveValueNodes(unavailableRawNodes(graph));
    if (!rawNodes.length) {
      setStatusText(unavailableEmptyMessage);
      return;
    }
    try {
      const response = await refreshLiveHoldings(rawNodes);
      mergeRefreshResult(response);
      setStatusText(onRefreshUnavailableSuccess(rawNodes.length, response));
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Unavailable live value check failed.");
    }
  }

  async function onLabelNode(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    const label = window.prompt("Enter label for this node:", "");
    if (label === null) {
      return;
    }
    await upsertAnnotation({ address, kind: "label", value: label });
    await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    setStatusText(`Saved label for ${address}.`);
  }

  async function onMarkAsgard(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    await upsertAnnotation({ address, kind: "asgard_vault", value: "true" });
    await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    setStatusText(`Marked ${address} as Asgard.`);
  }

  async function onRemoveNode(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const address = nodeAddressForActions(node, graph);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
      return;
    }
    await addToBlocklist({ address, reason: "Removed from graph" });
    await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
    setStatusText(`Removed ${address} from graph.`);
  }

  return {
    onOpenExplorer,
    onCopyAddress,
    onRefreshLiveValue,
    onRefreshUnavailable,
    onLabelNode,
    onMarkAsgard,
    onRemoveNode,
  };
}
