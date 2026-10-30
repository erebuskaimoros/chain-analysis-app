// Recognises what a pasted string is: a transaction hash, an address (with a
// best guess at its chain), or plain text to search for. The guess only labels
// and pre-fills; the server still decides which chains an address is active on.

export type IdentifiedInput =
  | { kind: "tx"; value: string }
  | { kind: "address"; value: string; chain: string; chainCertain: boolean }
  | { kind: "text"; value: string }
  | { kind: "empty"; value: "" };

const BASE58 = "1-9A-HJ-NP-Za-km-z";

const ADDRESS_PATTERNS: Array<{ chain: string; pattern: RegExp }> = [
  { chain: "THOR", pattern: /^(thor|sthor|tthor)1[0-9a-z]{38,58}$/ },
  { chain: "MAYA", pattern: /^(maya|smaya|tmaya)1[0-9a-z]{38,58}$/ },
  { chain: "GAIA", pattern: /^cosmos1[0-9a-z]{38,58}$/ },
  { chain: "XRD", pattern: /^account_(rdx|tdx_[0-9a-z]+_)1[0-9a-z]{20,}$/ },
  { chain: "BTC", pattern: /^(bc1[02-9ac-hj-np-z]{11,87}|[13][a-km-zA-HJ-NP-Z1-9]{25,34})$/ },
  { chain: "LTC", pattern: /^(ltc1[02-9ac-hj-np-z]{11,87}|[LM][a-km-zA-HJ-NP-Z1-9]{26,33})$/ },
  { chain: "BCH", pattern: /^(bitcoincash:)?[qp][02-9ac-hj-np-z]{41}$/ },
  { chain: "DOGE", pattern: new RegExp(`^D[5-9A-HJ-NP-U][${BASE58}]{32}$`) },
  { chain: "TRON", pattern: new RegExp(`^T[${BASE58}]{33}$`) },
  { chain: "XRP", pattern: new RegExp(`^r[${BASE58}]{24,34}$`) },
];

// EVM addresses look the same on every EVM chain, so the chain is a guess.
const EVM_ADDRESS = /^0x[0-9a-fA-F]{40}$/;
const SOLANA_ADDRESS = new RegExp(`^[${BASE58}]{32,44}$`);
const TX_HASH = /^(0x)?[0-9a-fA-F]{64}$/;

export const EVM_CHAINS = ["ETH", "BSC", "BASE", "AVAX", "ARB"];

export const KNOWN_CHAINS = [
  "THOR",
  "MAYA",
  "BTC",
  "ETH",
  "BSC",
  "BASE",
  "AVAX",
  "ARB",
  "LTC",
  "BCH",
  "DOGE",
  "GAIA",
  "SOL",
  "TRON",
  "XRP",
  "XRD",
];

export function guessAddressChain(address: string): { chain: string; certain: boolean } | null {
  const value = address.trim();
  if (!value) {
    return null;
  }
  for (const { chain, pattern } of ADDRESS_PATTERNS) {
    if (pattern.test(value)) {
      return { chain, certain: true };
    }
  }
  if (EVM_ADDRESS.test(value)) {
    return { chain: "ETH", certain: false };
  }
  if (SOLANA_ADDRESS.test(value)) {
    return { chain: "SOL", certain: true };
  }
  return null;
}

export function identifyInput(input: string): IdentifiedInput {
  const value = input.trim();
  if (!value) {
    return { kind: "empty", value: "" };
  }
  if (TX_HASH.test(value)) {
    return { kind: "tx", value };
  }
  // CHAIN|address states the chain outright.
  const piped = value.match(/^([A-Za-z0-9]{2,8})\|(\S+)$/);
  if (piped) {
    return { kind: "address", value: piped[2], chain: piped[1].toUpperCase(), chainCertain: true };
  }
  if (/\s/.test(value)) {
    return { kind: "text", value };
  }
  const guess = guessAddressChain(value);
  if (guess) {
    return { kind: "address", value, chain: guess.chain, chainCertain: guess.certain };
  }
  return { kind: "text", value };
}

// encodeSeed writes an address the way the trace and expansion APIs accept it.
export function encodeSeed(address: string, chain?: string) {
  const cleanAddress = address.trim();
  const cleanChain = (chain ?? "").trim().toUpperCase();
  return cleanChain ? `${cleanChain}|${cleanAddress}` : cleanAddress;
}
