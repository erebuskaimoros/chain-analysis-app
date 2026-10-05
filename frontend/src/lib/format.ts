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

// middleTruncate keeps both ends of an identifier, which is how people compare
// addresses and hashes by eye.
export function middleTruncate(value: string | undefined | null, head = 8, tail = 6) {
  const text = String(value ?? "").trim();
  if (text.length <= head + tail + 1) {
    return text;
  }
  return `${text.slice(0, head)}…${text.slice(-tail)}`;
}

export function formatRelativeTime(value: string | undefined | null, now = new Date()) {
  if (!value) {
    return "never";
  }
  const date = new Date(value);
  const time = date.getTime();
  if (Number.isNaN(time)) {
    return value;
  }
  const seconds = Math.round((now.getTime() - time) / 1000);
  if (seconds < 45) {
    return "just now";
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return `${minutes} min ago`;
  }
  const hours = Math.round(minutes / 60);
  if (hours < 24) {
    return `${hours} h ago`;
  }
  const days = Math.round(hours / 24);
  if (days === 1) {
    return "yesterday";
  }
  if (days < 7) {
    return `${days} days ago`;
  }
  return date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: date.getFullYear() === now.getFullYear() ? undefined : "numeric",
  });
}

export function formatDateRange(start: string | undefined | null, end: string | undefined | null) {
  const startDate = start ? new Date(start) : null;
  const endDate = end ? new Date(end) : null;
  const valid = (date: Date | null) => Boolean(date && !Number.isNaN(date.getTime()));
  if (!valid(startDate) || !valid(endDate)) {
    return "";
  }
  const sameYear = startDate!.getFullYear() === endDate!.getFullYear();
  const thisYear = endDate!.getFullYear() === new Date().getFullYear();
  const options: Intl.DateTimeFormatOptions = { month: "short", day: "numeric" };
  const startText = startDate!.toLocaleDateString(undefined, sameYear ? options : { ...options, year: "numeric" });
  const endText = endDate!.toLocaleDateString(undefined, thisYear && sameYear ? options : { ...options, year: "numeric" });
  return `${startText} – ${endText}`;
}

export function formatCompactUSD(value: number | undefined | null) {
  if (!Number.isFinite(value ?? NaN)) {
    return "n/a";
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    notation: Math.abs(value ?? 0) >= 100_000 ? "compact" : "standard",
    maximumFractionDigits: Math.abs(value ?? 0) >= 100 ? 0 : 2,
  }).format(value ?? 0);
}

export function pluralize(count: number, singular: string, plural = `${singular}s`) {
  return `${new Intl.NumberFormat("en-US").format(count)} ${count === 1 ? singular : plural}`;
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
