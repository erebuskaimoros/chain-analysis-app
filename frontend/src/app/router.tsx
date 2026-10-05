import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

export type ViewKey = "home" | "actors" | "graph" | "explorer" | "trace" | "cases" | "annotations";

export const VIEW_KEYS: ViewKey[] = ["home", "actors", "graph", "explorer", "trace", "cases", "annotations"];

// Older links used #overview for the landing page.
const VIEW_ALIASES: Record<string, ViewKey> = { overview: "home" };

export type RouteParams = Record<string, string>;

export interface RouteIntent {
  id: number;
  view: ViewKey;
  params: RouteParams;
}

interface RouterValue {
  view: ViewKey;
  intent: RouteIntent | null;
  navigate: (view: ViewKey, params?: RouteParams) => void;
  clearIntent: (id: number) => void;
}

let intentSeq = 0;
// Intents a page already acted on. Module level so React StrictMode's double
// effect pass in development does not run an intent twice.
const handledIntents = new Set<number>();

export function parseHash(hash: string): { view: ViewKey; params: RouteParams } {
  const raw = hash.replace(/^#/, "");
  const [path, query = ""] = raw.split("?", 2);
  const candidate = path.trim().toLowerCase();
  const view = (VIEW_KEYS as string[]).includes(candidate)
    ? (candidate as ViewKey)
    : VIEW_ALIASES[candidate] ?? "home";
  const params: RouteParams = {};
  new URLSearchParams(query).forEach((value, key) => {
    params[key] = value;
  });
  return { view, params };
}

export function buildHash(view: ViewKey, params: RouteParams = {}) {
  const query = new URLSearchParams(
    Object.entries(params).filter(([, value]) => value !== undefined && value !== "")
  ).toString();
  return query ? `#${view}?${query}` : `#${view}`;
}

function intentFor(view: ViewKey, params: RouteParams): RouteIntent | null {
  if (!Object.keys(params).length) {
    return null;
  }
  intentSeq += 1;
  return { id: intentSeq, view, params };
}

const RouterContext = createContext<RouterValue>({
  view: "home",
  intent: null,
  navigate: () => undefined,
  clearIntent: () => undefined,
});

export function RouterProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState(() => {
    const parsed = parseHash(window.location.hash);
    return { view: parsed.view, intent: intentFor(parsed.view, parsed.params) };
  });
  const lastHashRef = useRef(window.location.hash);

  useEffect(() => {
    function onHashChange() {
      if (window.location.hash === lastHashRef.current) {
        return;
      }
      lastHashRef.current = window.location.hash;
      const parsed = parseHash(window.location.hash);
      setState({ view: parsed.view, intent: intentFor(parsed.view, parsed.params) });
    }
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  const navigate = useCallback((view: ViewKey, params: RouteParams = {}) => {
    const hash = buildHash(view, params);
    lastHashRef.current = hash;
    if (window.location.hash !== hash) {
      window.location.hash = hash;
    }
    setState({ view, intent: intentFor(view, params) });
  }, []);

  const clearIntent = useCallback((id: number) => {
    setState((current) => {
      if (!current.intent || current.intent.id !== id) {
        return current;
      }
      const hash = buildHash(current.view);
      lastHashRef.current = hash;
      try {
        window.history.replaceState(window.history.state, "", hash);
      } catch {
        // History unavailable; the hash keeps its parameters.
      }
      return { view: current.view, intent: null };
    });
  }, []);

  const value = useMemo(
    () => ({ view: state.view, intent: state.intent, navigate, clearIntent }),
    [clearIntent, navigate, state.intent, state.view]
  );

  return <RouterContext.Provider value={value}>{children}</RouterContext.Provider>;
}

export function useRouter() {
  return useContext(RouterContext);
}

// useRouteIntent runs handler once for each set of parameters a link or the
// search palette sends to this view (for example #explorer?address=…), then
// strips the parameters from the address bar.
export function useRouteIntent(view: ViewKey, handler: (params: RouteParams) => void) {
  const { intent, clearIntent } = useRouter();
  const handlerRef = useRef(handler);
  handlerRef.current = handler;

  useEffect(() => {
    if (!intent || intent.view !== view || handledIntents.has(intent.id)) {
      return;
    }
    handledIntents.add(intent.id);
    handlerRef.current(intent.params);
    clearIntent(intent.id);
  }, [clearIntent, intent, view]);
}
