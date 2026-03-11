import type { ActorAddress, ActorAddressInput, AddressAnnotation } from "./types";

export interface ParsedActorAddresses {
  addresses: ActorAddressInput[];
  errors: string[];
}

export function parseActorAddressLines(text: string, annotations: AddressAnnotation[] = []): ParsedActorAddresses {
  const lines = text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  const addresses: ActorAddressInput[] = [];
  const errors: string[] = [];
  const seen = new Set<string>();
  const labelLookup = labelAnnotationLookup(annotations);

  lines.forEach((line, index) => {
    const parts = line
      .split(",")
      .map((part) => part.trim())
      .filter((part, partIndex) => part !== "" || partIndex === 0);
    if (!parts.length) {
      return;
    }

    const matches = labelLookup.get(parts[0].toLowerCase()) ?? [];
    if (matches.length > 1) {
      errors.push(`Ambiguous annotated address on line ${index + 1}: ${parts[0]}`);
      return;
    }

    const resolved = matches[0];
    const address = resolved?.address ?? parts[0];
    let chainHint = "";
    let label = resolved?.value ?? "";
    const remaining = resolved ? parts.slice(1) : parts.slice(1);

    if (remaining.length === 1) {
      if (/^[A-Za-z0-9_.-]{2,12}$/.test(remaining[0])) {
        chainHint = remaining[0].toUpperCase();
      } else {
        label = remaining[0];
      }
    }

    if (remaining.length >= 2) {
      chainHint = remaining[0].toUpperCase();
      label = remaining.slice(1).join(", ");
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

function labelAnnotationLookup(annotations: AddressAnnotation[]) {
  const lookup = new Map<string, AddressAnnotation[]>();
  annotations
    .filter((annotation) => annotation.kind === "label")
    .forEach((annotation) => {
      const key = annotation.value.trim().toLowerCase();
      if (!key) {
        return;
      }
      const matches = lookup.get(key) ?? [];
      matches.push(annotation);
      lookup.set(key, matches);
    });
  return lookup;
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
