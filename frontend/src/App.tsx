import { lazy, Suspense, useEffect, useState, type ComponentType, type LazyExoticComponent } from "react";
import { ActiveCaseProvider } from "./app/activeCase";
import { MenuIcon } from "./app/icons";
import { NAV_ITEMS } from "./app/navigation";
import { PageErrorBoundary } from "./app/PageErrorBoundary";
import { useMediaQuery, usePreference } from "./app/preferences";
import { Rail } from "./app/Rail";
import { RouterProvider, useRouter, type ViewKey } from "./app/router";
import { SearchPaletteProvider } from "./app/search";
import { ShortcutsDialog } from "./app/ShortcutsDialog";
import { ToastProvider } from "./app/toast";

type LazyPageComponent = LazyExoticComponent<ComponentType> & {
  preload: () => Promise<unknown>;
};

function lazyPage<T extends ComponentType<any>>(loader: () => Promise<{ default: T }>): LazyPageComponent {
  return Object.assign(lazy(loader), { preload: loader });
}

const pageComponents: Record<ViewKey, LazyPageComponent> = {
  home: lazyPage(() => import("./features/home/HomePage").then((module) => ({ default: module.HomePage }))),
  actors: lazyPage(() => import("./features/actors/ActorsPage").then((module) => ({ default: module.ActorsPage }))),
  graph: lazyPage(() =>
    import("./features/actor-graph/ActorGraphPage").then((module) => ({ default: module.ActorGraphPage }))
  ),
  explorer: lazyPage(() => import("./features/explorer/ExplorerPage").then((module) => ({ default: module.ExplorerPage }))),
  trace: lazyPage(() => import("./features/trace/TracePage").then((module) => ({ default: module.TracePage }))),
  cases: lazyPage(() => import("./features/cases/CasesPage").then((module) => ({ default: module.CasesPage }))),
  annotations: lazyPage(() =>
    import("./features/annotations/AnnotationsPage").then((module) => ({ default: module.AnnotationsPage }))
  ),
};

type RailPreference = "auto" | "expanded" | "collapsed";

function Shell() {
  const { view } = useRouter();
  const [railPreference, setRailPreference] = usePreference<RailPreference>("chain-analysis.rail", "auto");
  const isMedium = useMediaQuery("(max-width: 1199px)");
  const isPhone = useMediaQuery("(max-width: 699px)");
  const [navOpen, setNavOpen] = useState(false);
  const current = NAV_ITEMS.find((item) => item.key === view) ?? NAV_ITEMS[0];
  const collapsed =
    !isPhone &&
    (isMedium || railPreference === "collapsed" || (railPreference === "auto" && Boolean(current.workspace)));
  const ActivePage = pageComponents[view];

  useEffect(() => {
    document.title = `${current.label} - Chain Analysis`;
  }, [current.label]);

  useEffect(() => {
    // Warm the heavier analysis pages in the background after first paint.
    const timer = window.setTimeout(() => {
      void pageComponents.graph.preload();
      void pageComponents.explorer.preload();
      void pageComponents.trace.preload();
    }, 1500);
    return () => window.clearTimeout(timer);
  }, []);

  return (
    <div className={`app${collapsed ? " is-rail-collapsed" : ""}${navOpen ? " is-nav-open" : ""}`}>
      {isPhone ? (
        <div className="topbar">
          <button
            type="button"
            className="btn btn-ghost btn-icon"
            aria-label="Open navigation"
            aria-expanded={navOpen}
            onClick={() => setNavOpen((value) => !value)}
          >
            <MenuIcon />
          </button>
          <span className="topbar-title">Chain Analysis</span>
        </div>
      ) : null}
      {isPhone && navOpen ? <div className="scrim" onClick={() => setNavOpen(false)} /> : null}
      <Rail
        collapsed={collapsed}
        onToggleCollapsed={() => setRailPreference(collapsed ? "expanded" : "collapsed")}
        onNavigate={() => setNavOpen(false)}
      />
      <main className="app-main" id="main">
        <PageErrorBoundary key={view} pageLabel={current.label}>
          <Suspense fallback={<PageLoading label={current.label} />}>
            <ActivePage />
          </Suspense>
        </PageErrorBoundary>
      </main>
      <ShortcutsDialog />
    </div>
  );
}

function PageLoading({ label }: { label: string }) {
  return (
    <header className="page-header">
      <div className="page-header-title">
        <h1>{label}</h1>
        <span className="page-header-context">Loading…</span>
      </div>
    </header>
  );
}

function App() {
  return (
    <RouterProvider>
      <ToastProvider>
        <ActiveCaseProvider>
          <SearchPaletteProvider>
            <Shell />
          </SearchPaletteProvider>
        </ActiveCaseProvider>
      </ToastProvider>
    </RouterProvider>
  );
}

export default App;
