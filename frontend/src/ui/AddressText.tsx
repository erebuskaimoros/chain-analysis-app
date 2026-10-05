import { useEffect, useState } from "react";
import { CheckIcon, CopyIcon } from "../app/icons";
import { CHAIN_LOGO_URLS } from "../lib/graph/types";
import { middleTruncate } from "../lib/format";

interface AddressTextProps {
  value: string;
  head?: number;
  tail?: number;
  // Show the whole value instead of truncating the middle.
  full?: boolean;
  copyable?: boolean;
  className?: string;
}

// AddressText shows an address or hash in monospace with its middle elided.
// Clicking copies the whole value.
export function AddressText({ value, head = 8, tail = 6, full = false, copyable = true, className = "" }: AddressTextProps) {
  const [copied, setCopied] = useState(false);
  const text = full ? value : middleTruncate(value, head, tail);

  useEffect(() => {
    if (!copied) {
      return;
    }
    const timer = window.setTimeout(() => setCopied(false), 1400);
    return () => window.clearTimeout(timer);
  }, [copied]);

  if (!value) {
    return <span className={`addr ${className}`}>n/a</span>;
  }

  if (!copyable) {
    return (
      <span className={`addr ${className}`} title={value}>
        {text}
      </span>
    );
  }

  return (
    <button
      type="button"
      className={`addr${copied ? " is-copied" : ""} ${className}`}
      title={`${value}\nClick to copy`}
      aria-label={copied ? `Copied ${value}` : `Copy ${value}`}
      onClick={(event) => {
        event.stopPropagation();
        void navigator.clipboard?.writeText(value).then(
          () => setCopied(true),
          () => undefined
        );
      }}
    >
      <span>{text}</span>
      <span className="addr-copy">{copied ? <CheckIcon size={12} /> : <CopyIcon size={12} />}</span>
    </button>
  );
}

export function ChainBadge({ chain, count }: { chain: string; count?: number }) {
  const upper = chain.toUpperCase();
  const logo = CHAIN_LOGO_URLS[upper];
  return (
    <span className="chain-badge" title={count === undefined ? upper : `${count} on ${upper}`}>
      {logo ? <img src={logo} alt="" loading="lazy" /> : null}
      <span>{upper || "Unknown"}</span>
      {count !== undefined ? <span className="chain-badge-count">{count}</span> : null}
    </span>
  );
}
