import {
  lazy,
  Suspense,
  useEffect,
  useMemo,
  useState,
  type ComponentType,
  type LazyExoticComponent,
} from "react";

type ViewKey = "overview" | "actors" | "graph" | "explorer" | "annotations";

interface ViewDef {
  key: ViewKey;
  label: string;
  eyebrow: string;
  title: string;
  description: string;
}

const views: ViewDef[] = [
  {
    key: "overview",
    label: "Overview",
    eyebrow: "System",
    title: "Liquidity Flow Workspace",
    description: "The typed React frontend drives the unified THOR + MAYA flow-analysis workflow directly against `/api/v1`, with `/legacy/` left untouched.",
  },
  {
    key: "actors",
    label: "Actors",
    eyebrow: "CRUD",
    title: "Actor Directory",
    description: "Create, edit, and delete actor definitions and their address sets from the native UI.",
  },
  {
    key: "graph",
    label: "Actor Graph",
    eyebrow: "Analysis",
    title: "Actor-Centered Flow Graph",
    description: "Build merged liquidity graphs, replay saved runs, expand nodes one hop, and refresh live holdings without leaving the typed workspace.",
  },
  {
    key: "explorer",
    label: "Explorer",
    eyebrow: "Analysis",
    title: "Address Explorer",
    description: "Preview address activity, choose loading direction when needed, and page explorer graph batches natively.",
  },
  {
    key: "annotations",
    label: "Annotations",
    eyebrow: "Metadata",
    title: "Annotations and Blocklist",
    description: "Manage labels, special address kinds, and blocklisted addresses from the same typed UI surface.",
  },
];

type LazyPageComponent = LazyExoticComponent<ComponentType> & {
  preload: () => Promise<unknown>;
};

function lazyPage<T extends ComponentType<any>>(loader: () => Promise<{ default: T }>): LazyPageComponent {
  return Object.assign(lazy(loader), { preload: loader });
}

const pageComponents: Record<ViewKey, LazyPageComponent> = {
  overview: lazyPage(() =>
    import("./features/overview/OverviewPage").then((module) => ({ default: module.OverviewPage }))
  ),
  actors: lazyPage(() =>
    import("./features/actors/ActorsPage").then((module) => ({ default: module.ActorsPage }))
  ),
  graph: lazyPage(() =>
    import("./features/actor-graph/ActorGraphPage").then((module) => ({ default: module.ActorGraphPage }))
  ),
  explorer: lazyPage(() =>
    import("./features/explorer/ExplorerPage").then((module) => ({ default: module.ExplorerPage }))
  ),
  annotations: lazyPage(() =>
    import("./features/annotations/AnnotationsPage").then((module) => ({ default: module.AnnotationsPage }))
  ),
};

function viewFromHash(hash: string): ViewKey {
  const candidate = hash.replace(/^#/, "").trim().toLowerCase();
  if (views.some((view) => view.key === candidate)) {
    return candidate as ViewKey;
  }
  return "overview";
}

function App() {
  const [activeView, setActiveView] = useState<ViewKey>(() => viewFromHash(window.location.hash));

  useEffect(() => {
    const onHashChange = () => {
      setActiveView(viewFromHash(window.location.hash));
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  function navigate(view: ViewKey) {
    window.location.hash = view;
    setActiveView(view);
  }

  const currentView = useMemo(
    () => views.find((view) => view.key === activeView) ?? views[0],
    [activeView]
  );
  const ActivePage = pageComponents[activeView];

  function preloadView(view: ViewKey) {
    void pageComponents[view].preload();
  }

  return (
    <div className="shell">
      <aside className="shell-sidebar">
        <div className="brand-block">
          <span className="eyebrow">Liquidity Flow</span>
          <h1>Chain Analysis</h1>
          <p>Typed backend routes and native React screens now analyze THOR, MAYA, and connected-chain flows end to end.</p>
        </div>
        <nav className="nav-list" aria-label="Primary">
          {views.map((view) => (
            <button
              key={view.key}
              type="button"
              className={`nav-item ${view.key === activeView ? "active" : ""}`}
              onClick={() => navigate(view.key)}
              onMouseEnter={() => preloadView(view.key)}
              onFocus={() => preloadView(view.key)}
            >
              <span>{view.label}</span>
              <small>{view.eyebrow}</small>
            </button>
          ))}
        </nav>
        <a className="link-button secondary" href="/legacy/" target="_blank" rel="noreferrer">
          Open Legacy App
        </a>
      </aside>

      <main className="shell-main">
        <section className="hero panel">
          <span className="eyebrow">{currentView.eyebrow}</span>
          <h2>{currentView.title}</h2>
          <p>{currentView.description}</p>
        </section>
        <Suspense
          fallback={
            <section className="panel page-panel">
              <span className="eyebrow">Loading</span>
              <h2>Loading {currentView.label}</h2>
              <p>Fetching the code bundle for this workspace view.</p>
            </section>
          }
        >
          <ActivePage />
        </Suspense>
      </main>
    </div>
  );
}

export default App;
