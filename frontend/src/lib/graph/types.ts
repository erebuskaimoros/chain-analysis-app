import type { AddressAnnotation, BlocklistedAddress, FlowEdge, FlowNode } from "../types";

export const CHAIN_LOGO_URLS: Record<string, string> = {
  THOR: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/thorchain/info/logo.png",
  MAYA: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/mayachain/info/logo.png",
  BTC: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/bitcoin/info/logo.png",
  ETH: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/ethereum/info/logo.png",
  ARB: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/arbitrum/info/logo.png",
  BSC: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/smartchain/info/logo.png",
  BASE: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/base/info/logo.png",
  AVAX: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/avalanchec/info/logo.png",
  LTC: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/litecoin/info/logo.png",
  BCH: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/bitcoincash/info/logo.png",
  DOGE: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/doge/info/logo.png",
  GAIA: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/cosmos/info/logo.png",
  SOL: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/solana/info/logo.png",
  TRON: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/tron/info/logo.png",
  XRP: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/ripple/info/logo.png",
  XRD: "https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/radix/info/logo.png",
};

export const GRAPH_FILTER_TXN_TYPES = [
  { key: "bond_unbond", label: "Bond/Unbond" },
  { key: "rebond", label: "Rebond" },
  { key: "transfer", label: "Send/Transfer" },
  { key: "swap", label: "Swap" },
] as const;

export type GraphTxnBucket = (typeof GRAPH_FILTER_TXN_TYPES)[number]["key"];

export interface GraphFilterState {
  initialized: boolean;
  isOpen: boolean;
  txnTypes: Record<GraphTxnBucket, boolean>;
  availableChains: string[];
  selectedChains: string[];
  graphMinTime: string;
  graphMaxTime: string;
  graphMinTxnUSD: number | null;
  graphMaxTxnUSD: number | null;
  startTime: string;
  endTime: string;
  minTxnUSD: number | null;
  maxTxnUSD: number | null;
}

export interface GraphMetadata {
  annotations: AddressAnnotation[];
  blocklist: BlocklistedAddress[];
}

export interface ActorGraphViewState {
  expandedActorIDs: number[];
  expandedExternalChains: string[];
}

export interface VisibleGraphNode extends FlowNode {
  raw_node_ids: string[];
  displayLabel: string;
  live_holdings_label: string;
  live_holdings_status: string;
  color: string;
  borderColor: string;
  chainLogo: string;
  inspect: Record<string, unknown>;
  pie1Color?: string;
  pie1Size?: number;
  pie2Color?: string;
  pie2Size?: number;
  pie3Color?: string;
  pie3Size?: number;
  pie4Color?: string;
  pie4Size?: number;
}

export interface VisibleGraphEdge extends FlowEdge {
  source: string;
  target: string;
  width: number;
  lineColor: string;
  edgeLabel: string;
  action_classes: string[];
  action_keys: string[];
  action_labels: string[];
  action_domains: string[];
  action_buckets: string[];
  validator_addresses: string[];
  validator_labels: string[];
  contract_types: string[];
  contract_protocols: string[];
  chain_set: string[];
  raw_edge_ids: string[];
  inspect: Record<string, unknown>;
}

export interface VisibleGraph {
  nodes: VisibleGraphNode[];
  edges: VisibleGraphEdge[];
}

export type GraphSelection =
  | { kind: "node"; node: VisibleGraphNode }
  | { kind: "nodes"; nodes: VisibleGraphNode[] }
  | { kind: "edge"; edge: VisibleGraphEdge }
  | null;

export type CytoscapeStyleBlock = {
  selector: string;
  style: Record<string, string | number>;
};
