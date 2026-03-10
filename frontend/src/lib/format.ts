export function formatDateTime(value: string | undefined | null) {
  if (!value) {
    return "Unknown";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

export function formatShortDateTime(value: string | undefined | null) {
  if (!value) {
    return "Unknown";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
}

export function toLocalInputValue(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return [
    date.getFullYear(),
    "-",
    pad(date.getMonth() + 1),
    "-",
    pad(date.getDate()),
    "T",
    pad(date.getHours()),
    ":",
    pad(date.getMinutes()),
  ].join("");
}

export function formatUSD(value: number | undefined | null) {
  if (!Number.isFinite(value ?? NaN)) {
    return "n/a";
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: value && Math.abs(value) >= 100 ? 0 : 2,
  }).format(value ?? 0);
}

export function shortHash(value: string | undefined | null) {
  const text = String(value ?? "").trim();
  if (text.length <= 18) {
    return text || "n/a";
  }
  return `${text.slice(0, 8)}...${text.slice(-8)}`;
}

export function prettyJSON(value: unknown) {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function normalizeAddressText(value: string) {
  return value.trim();
}
