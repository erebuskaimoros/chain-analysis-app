import { useEffect, useMemo, useState } from "react";
import { ActorsPage } from "./features/actors/ActorsPage";
import { ActorGraphPage } from "./features/actor-graph/ActorGraphPage";
import { AnnotationsPage } from "./features/annotations/AnnotationsPage";
import { ExplorerPage } from "./features/explorer/ExplorerPage";
import { OverviewPage } from "./features/overview/OverviewPage";

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
        {activeView === "overview" ? <OverviewPage /> : null}
        {activeView === "actors" ? <ActorsPage /> : null}
        {activeView === "graph" ? <ActorGraphPage /> : null}
        {activeView === "explorer" ? <ExplorerPage /> : null}
        {activeView === "annotations" ? <AnnotationsPage /> : null}
      </main>
    </div>
  );
}

export default App;
