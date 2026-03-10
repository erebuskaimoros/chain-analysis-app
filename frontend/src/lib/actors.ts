import type { ActorAddress, ActorAddressInput } from "./types";

export interface ParsedActorAddresses {
  addresses: ActorAddressInput[];
  errors: string[];
}

export function parseActorAddressLines(text: string): ParsedActorAddresses {
  const lines = text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  const addresses: ActorAddressInput[] = [];
  const errors: string[] = [];
  const seen = new Set<string>();

  lines.forEach((line, index) => {
    const parts = line
      .split(",")
      .map((part) => part.trim())
      .filter((part, partIndex) => part !== "" || partIndex === 0);
    if (!parts.length) {
      return;
    }

    const address = parts[0];
    let chainHint = "";
    let label = "";

    if (parts.length === 2) {
      if (/^[A-Za-z0-9_.-]{2,12}$/.test(parts[1])) {
        chainHint = parts[1].toUpperCase();
      } else {
        label = parts[1];
      }
    }

    if (parts.length >= 3) {
      chainHint = parts[1].toUpperCase();
      label = parts.slice(2).join(", ");
    }

    const key = address.toLowerCase();
    if (seen.has(key)) {
      errors.push(`Duplicate address on line ${index + 1}: ${address}`);
      return;
    }

    seen.add(key);
    addresses.push({
      address,
      chain_hint: chainHint,
      label,
    });
  });

  return { addresses, errors };
}

export function formatActorAddressLines(addresses: ActorAddress[]) {
  return addresses
    .map((item) => {
      const parts = [item.address];
      if (item.chain_hint) {
        parts.push(item.chain_hint);
      }
      if (item.label) {
        if (!item.chain_hint) {
          parts.push("");
        }
        parts.push(item.label);
      }
      return parts.join(",");
    })
    .join("\n");
}

export function encodeActorAddressSeed(address: string, chainHint: string) {
  const cleanAddress = address.trim();
  const cleanChain = chainHint.trim().toUpperCase();
  if (!cleanAddress) {
    return "";
  }
  return cleanChain ? `${cleanChain}|${cleanAddress}` : cleanAddress;
}
