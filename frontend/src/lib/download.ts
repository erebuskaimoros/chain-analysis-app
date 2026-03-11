function sanitizeFilenamePart(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);
}

export function buildDownloadTimestamp(date = new Date()) {
  return date.toISOString().replace(/[:.]/g, "-");
}

export function buildGraphStateFilename(prefix: string, hint = "") {
  const cleanHint = sanitizeFilenamePart(hint);
  const parts = [sanitizeFilenamePart(prefix) || "graph-state"];
  if (cleanHint) {
    parts.push(cleanHint);
  }
  parts.push(buildDownloadTimestamp());
  return `${parts.join("-")}.json`;
}

export function downloadJSON(filename: string, value: unknown) {
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
  return filename;
}
