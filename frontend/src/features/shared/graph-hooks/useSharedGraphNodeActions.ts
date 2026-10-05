import type { QueryClient } from "@tanstack/react-query";
import type { Dispatch, SetStateAction } from "react";
import {
  addToBlocklist,
  refreshLiveHoldingsInBackground,
  removeFromBlocklist,
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
import { middleTruncate } from "../../../lib/format";
import { encodeSeed } from "../../../lib/identify";
import type {
  ActorGraphResponse,
  AddressExplorerResponse,
  LiveHoldingsRefreshResponse,
} from "../../../lib/types";
import { useActiveCase } from "../../../app/activeCase";
import { useRouter } from "../../../app/router";
import { useToast } from "../../../app/toast";

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
  const toast = useToast();
  const { navigate } = useRouter();
  const { addToCase } = useActiveCase();

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

  // resolveAddress returns the node's single address, or "" for clusters and
  // nodes that stand for several addresses.
  function resolveAddress(node: VisibleGraphNode) {
    return graph ? nodeAddressForActions(node, graph) : "";
  }

  function requireAddress(node: VisibleGraphNode) {
    const address = resolveAddress(node);
    if (!address) {
      setStatusText("Selected node does not resolve to a single address.");
    }
    return address;
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
    const address = requireAddress(node);
    if (!address) {
      return;
    }
    await navigator.clipboard.writeText(address);
    setStatusText(`Copied: ${address}`);
    toast(`Copied ${middleTruncate(address)}`);
  }

  function onExploreAddress(node: VisibleGraphNode) {
    const address = requireAddress(node);
    if (address) {
      navigate("explorer", { address });
    }
  }

  function onTraceFrom(node: VisibleGraphNode) {
    const address = requireAddress(node);
    if (address) {
      navigate("trace", { seed: encodeSeed(address, node.chain) });
    }
  }

  function onAddToCase(node: VisibleGraphNode) {
    const address = requireAddress(node);
    if (address) {
      addToCase([{ kind: "address", ref: encodeSeed(address, node.chain) }], node.displayLabel || middleTruncate(address));
    }
  }

  function onAddNodesToCase(nodes: VisibleGraphNode[]) {
    const items = nodes
      .map((node) => ({ node, address: resolveAddress(node) }))
      .filter((entry) => entry.address)
      .map((entry) => ({ kind: "address" as const, ref: encodeSeed(entry.address, entry.node.chain) }));
    if (!items.length) {
      setStatusText("None of the selected nodes resolve to a single address.");
      return;
    }
    addToCase(items, items.length === 1 ? "1 address" : `${items.length} addresses`);
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
      const response = await refreshLiveHoldingsInBackground(refreshableNodes, { force: true });
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
      const response = await refreshLiveHoldingsInBackground(rawNodes, { force: true });
      mergeRefreshResult(response);
      setStatusText(onRefreshUnavailableSuccess(rawNodes.length, response));
    } catch (error) {
      setStatusText(error instanceof Error ? error.message : "Unavailable live value check failed.");
    }
  }

  // saveLabel stores a label annotation for the node's address. The graph
  // shows it the next time annotations load.
  async function saveLabel(node: VisibleGraphNode, label: string) {
    if (!graph) {
      return false;
    }
    const address = requireAddress(node);
    if (!address) {
      return false;
    }
    try {
      await upsertAnnotation({ address, kind: "label", value: label.trim() });
      await queryClient.invalidateQueries({ queryKey: ["annotations"] });
      setStatusText(`Saved label for ${address}.`);
      toast(`Labeled ${middleTruncate(address)} as ${label.trim()}`);
      return true;
    } catch (error) {
      toast(error instanceof Error ? `Could not save the label: ${error.message}` : "Could not save the label.", { tone: "error" });
      return false;
    }
  }

  async function onMarkAsgard(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const address = requireAddress(node);
    if (!address) {
      return;
    }
    await upsertAnnotation({ address, kind: "asgard_vault", value: "true" });
    await queryClient.invalidateQueries({ queryKey: ["annotations"] });
    setStatusText(`Marked ${address} as Asgard.`);
    toast(`Marked ${middleTruncate(address)} as an Asgard vault`);
  }

  // onRemoveNode excludes the address from every graph (the blocklist), with
  // an undo in the confirmation.
  async function onRemoveNode(node: VisibleGraphNode) {
    if (!graph) {
      return;
    }
    const address = requireAddress(node);
    if (!address) {
      return;
    }
    await addToBlocklist({ address, reason: "Removed from graph" });
    await queryClient.invalidateQueries({ queryKey: ["blocklist"] });
    setStatusText(`Removed ${address} from graph.`);
    toast(`Excluded ${middleTruncate(address)} from all graphs`, {
      actionLabel: "Undo",
      onAction: () => {
        void removeFromBlocklist(address).then(() => queryClient.invalidateQueries({ queryKey: ["blocklist"] }));
      },
    });
  }

  return {
    resolveAddress,
    onOpenExplorer,
    onCopyAddress,
    onExploreAddress,
    onTraceFrom,
    onAddToCase,
    onAddNodesToCase,
    onRefreshLiveValue,
    onRefreshUnavailable,
    saveLabel,
    onMarkAsgard,
    onRemoveNode,
  };
}
