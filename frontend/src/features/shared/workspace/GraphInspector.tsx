import { useEffect, useState, type FormEvent } from "react";
import {
  ExplorerIcon,
  ExternalIcon,
  GraphIcon,
  PinIcon,
  TraceIcon,
} from "../../../app/icons";
import { useActiveCase } from "../../../app/activeCase";
import { useRouter } from "../../../app/router";
import { middleTruncate } from "../../../lib/format";
import type { GraphSelection, VisibleGraphNode } from "../../../lib/graph";
import { explorerURLForTx } from "../../../lib/graph/actions";
import type { ActionLookupResponse } from "../../../lib/types";
import { Field } from "../../../ui/Field";
import { MenuButton, type MenuItem } from "../../../ui/Menu";
import { ActionLookupPanel } from "../ActionLookupPanel";
import { SelectionInspector } from "../SelectionInspector";
import type { useSharedGraphNodeActions } from "../graph-hooks/useSharedGraphNodeActions";

export type InspectorTab = "details" | "tx";

type NodeActions = ReturnType<typeof useSharedGraphNodeActions>;

interface GraphInspectorProps {
  selection: GraphSelection;
  tab: InspectorTab;
  lookup: { result: ActionLookupResponse | null; isLoading: boolean; error: string; txID: string };
  onLookupTx: (txID: string) => void;
  nodeLabel: (id: string) => string;
  actorName?: (id: number) => string;
  nodeActions: NodeActions;
  onExpandNode: (node: VisibleGraphNode) => void;
  onExpandNodes: (nodes: VisibleGraphNode[]) => void;
  labelingNodeID: string | null;
  onLabelingChange: (nodeID: string | null) => void;
  existingLabel: (address: string) => string;
  expandLabel?: string;
}

export function inspectorTitle(selection: GraphSelection, tab: InspectorTab) {
  if (tab === "tx") {
    return "Transaction";
  }
  if (!selection) {
    return "Inspector";
  }
  if (selection.kind === "edge") {
    return "Flow";
  }
  if (selection.kind === "nodes") {
    return "Selection";
  }
  return "Address";
}

export function InspectorTabs({
  tab,
  onTabChange,
  hasLookup,
}: {
  tab: InspectorTab;
  onTabChange: (tab: InspectorTab) => void;
  hasLookup: boolean;
}) {
  if (!hasLookup) {
    return null;
  }
  return (
    <div className="inspector-tabs" role="tablist" aria-label="Inspector">
      <button type="button" role="tab" aria-selected={tab === "details"} onClick={() => onTabChange("details")}>
        Selection
      </button>
      <button type="button" role="tab" aria-selected={tab === "tx"} onClick={() => onTabChange("tx")}>
        Transaction
      </button>
    </div>
  );
}

export function GraphInspector({
  selection,
  tab,
  lookup,
  onLookupTx,
  nodeLabel,
  actorName,
  nodeActions,
  onExpandNode,
  onExpandNodes,
  labelingNodeID,
  onLabelingChange,
  existingLabel,
  expandLabel = "Expand one edge",
}: GraphInspectorProps) {
  const { addToCase } = useActiveCase();
  const { navigate } = useRouter();

  if (tab === "tx") {
    const txID = lookup.result?.tx_id || lookup.txID;
    return (
      <div className="section">
        {txID ? (
          <div className="inspector-actions">
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => addToCase([{ kind: "tx", ref: txID }], middleTruncate(txID))}
            >
              <PinIcon />
              Add to case
            </button>
            <button type="button" className="btn btn-sm" onClick={() => navigate("trace", { seed: txID })}>
              <TraceIcon />
              Trace from here
            </button>
            <a className="btn btn-sm" href={explorerURLForTx(txID, "THOR")} target="_blank" rel="noreferrer">
              <ExternalIcon />
              thorchain.net
            </a>
          </div>
        ) : null}
        <ActionLookupPanel result={lookup.result} isLoading={lookup.isLoading} error={lookup.error} />
      </div>
    );
  }

  let actions = null;
  if (selection?.kind === "node") {
    const node = selection.node;
    const address = nodeActions.resolveAddress(node);
    const more: MenuItem[] = [];
    if (address) {
      more.push(
        { label: "Label this address…", onSelect: () => onLabelingChange(node.id) },
        { label: "Copy address", onSelect: () => void nodeActions.onCopyAddress(node) },
        { label: "Open in a block explorer", icon: <ExternalIcon />, onSelect: () => void nodeActions.onOpenExplorer(node) }
      );
    }
    more.push({ label: "Refresh live value", onSelect: () => void nodeActions.onRefreshLiveValue(node) });
    if (address) {
      more.push(
        { label: "Mark as Asgard vault", onSelect: () => void nodeActions.onMarkAsgard(node) },
        "separator",
        { label: "Exclude from all graphs", danger: true, onSelect: () => void nodeActions.onRemoveNode(node) }
      );
    }
    actions = (
      <>
        <button type="button" className="btn btn-sm" onClick={() => onExpandNode(node)}>
          <GraphIcon />
          {expandLabel}
        </button>
        {address ? (
          <>
            <button type="button" className="btn btn-sm" onClick={() => nodeActions.onExploreAddress(node)}>
              <ExplorerIcon />
              Explore
            </button>
            <button type="button" className="btn btn-sm" onClick={() => nodeActions.onTraceFrom(node)}>
              <TraceIcon />
              Trace
            </button>
            <button type="button" className="btn btn-sm" onClick={() => nodeActions.onAddToCase(node)}>
              <PinIcon />
              Add to case
            </button>
          </>
        ) : null}
        <MenuButton label="More actions for this node" items={more} className="btn btn-sm btn-icon" />
      </>
    );
  } else if (selection?.kind === "nodes") {
    const nodes = selection.nodes;
    actions = (
      <>
        <button type="button" className="btn btn-sm" onClick={() => onExpandNodes(nodes)}>
          <GraphIcon />
          Expand selected
        </button>
        <button type="button" className="btn btn-sm" onClick={() => nodeActions.onAddNodesToCase(nodes)}>
          <PinIcon />
          Add {nodes.length} to case
        </button>
      </>
    );
  } else if (selection?.kind === "edge") {
    const edge = selection.edge;
    const txIDs = Array.from(new Set(edge.tx_ids)).filter(Boolean);
    actions = txIDs.length ? (
      <button
        type="button"
        className="btn btn-sm"
        onClick={() =>
          addToCase(
            txIDs.map((txID) => ({ kind: "tx" as const, ref: txID })),
            txIDs.length === 1 ? middleTruncate(txIDs[0]) : `${txIDs.length} transactions`
          )
        }
      >
        <PinIcon />
        {txIDs.length === 1 ? "Add transaction to case" : `Add ${txIDs.length} transactions to case`}
      </button>
    ) : null;
  }

  const labelingNode = selection?.kind === "node" && labelingNodeID === selection.node.id ? selection.node : null;

  return (
    <div className="section">
      {labelingNode ? (
        <LabelForm
          node={labelingNode}
          address={nodeActions.resolveAddress(labelingNode)}
          initial={existingLabel(nodeActions.resolveAddress(labelingNode))}
          onSave={async (label) => {
            const saved = await nodeActions.saveLabel(labelingNode, label);
            if (saved) {
              onLabelingChange(null);
            }
          }}
          onCancel={() => onLabelingChange(null)}
        />
      ) : null}
      <SelectionInspector
        selection={selection}
        emptyMessage="Click a node or a flow on the map to see its details here."
        onLookupTx={onLookupTx}
        nodeLabel={nodeLabel}
        actorName={actorName}
        actions={actions}
      />
    </div>
  );
}

function LabelForm({
  node,
  address,
  initial,
  onSave,
  onCancel,
}: {
  node: VisibleGraphNode;
  address: string;
  initial: string;
  onSave: (label: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [value, setValue] = useState(initial);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setValue(initial);
  }, [initial, node.id]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!value.trim()) {
      return;
    }
    setBusy(true);
    await onSave(value);
    setBusy(false);
  }

  return (
    <form className="section" onSubmit={(event) => void submit(event)} aria-label="Label this address">
      <Field label={`Label for ${middleTruncate(address)}`} hint="Labels show on the graph and in search. Manage them on the Labels page.">
        <input
          className="input"
          value={value}
          autoFocus
          placeholder="Treasury hot wallet"
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.stopPropagation();
              onCancel();
            }
          }}
        />
      </Field>
      <div className="form-actions">
        <button type="submit" className="btn btn-primary btn-sm" disabled={busy || !value.trim()}>
          Save label
        </button>
        <button type="button" className="btn btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}
