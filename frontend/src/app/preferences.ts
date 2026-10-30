import { useCallback, useEffect, useState } from "react";

// Per-viewer conveniences (theme, panel sizes, the active case). Storage can be
// unavailable in private windows, so every read and write tolerates failure.
export function readPreference<T>(key: string, fallback: T): T {
  try {
    const raw = window.localStorage.getItem(key);
    return raw === null ? fallback : (JSON.parse(raw) as T);
  } catch {
    return fallback;
  }
}

export function writePreference<T>(key: string, value: T) {
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // The preference stays for this session only.
  }
}

export function usePreference<T>(key: string, fallback: T) {
  const [value, setValue] = useState<T>(() => readPreference(key, fallback));
  const update = useCallback(
    (next: T | ((current: T) => T)) => {
      setValue((current) => {
        const resolved = typeof next === "function" ? (next as (current: T) => T)(current) : next;
        writePreference(key, resolved);
        return resolved;
      });
    },
    [key]
  );
  return [value, update] as const;
}

export function useMediaQuery(query: string) {
  const [matches, setMatches] = useState(() =>
    typeof window !== "undefined" && typeof window.matchMedia === "function" ? window.matchMedia(query).matches : false
  );

  useEffect(() => {
    if (typeof window.matchMedia !== "function") {
      return;
    }
    const list = window.matchMedia(query);
    const onChange = () => setMatches(list.matches);
    onChange();
    list.addEventListener?.("change", onChange);
    return () => list.removeEventListener?.("change", onChange);
  }, [query]);

  return matches;
}

export type ThemePreference = "system" | "light" | "dark";

export function useThemePreference() {
  const [theme, setTheme] = usePreference<ThemePreference>("chain-analysis.theme", "system");

  useEffect(() => {
    const root = document.documentElement;
    if (theme === "system") {
      delete root.dataset.theme;
    } else {
      root.dataset.theme = theme;
    }
  }, [theme]);

  return [theme, setTheme] as const;
}
