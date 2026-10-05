import { useQuery } from "@tanstack/react-query";
import { listActors, listAnnotations } from "../lib/api";
import { pluralize } from "../lib/format";
import { HealthIndicator } from "../features/health/HealthPanel";
import { useActiveCase } from "./activeCase";
import {
  BrandMark,
  CasesIcon,
  CollapseIcon,
  ExpandIcon,
  KeyboardIcon,
  MonitorIcon,
  MoonIcon,
  SearchIcon,
  SunIcon,
} from "./icons";
import { NAV_ITEMS } from "./navigation";
import { useThemePreference, type ThemePreference } from "./preferences";
import { useRouter, type ViewKey } from "./router";
import { useSearchPalette } from "./search";
import { openShortcuts } from "./ShortcutsDialog";

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

const NEXT_THEME: Record<ThemePreference, ThemePreference> = { system: "light", light: "dark", dark: "system" };
const THEME_LABEL: Record<ThemePreference, string> = {
  system: "Theme follows the system",
  light: "Light theme",
  dark: "Dark theme",
};

interface RailProps {
  collapsed: boolean;
  onToggleCollapsed: () => void;
  onNavigate?: () => void;
}

export function Rail({ collapsed, onToggleCollapsed, onNavigate }: RailProps) {
  const { view, navigate } = useRouter();
  const { openSearch } = useSearchPalette();
  const { activeCase, cases, chooseCase } = useActiveCase();
  const [theme, setTheme] = useThemePreference();
  const actorsQuery = useQuery({ queryKey: ["actors"], queryFn: listActors });
  const annotationsQuery = useQuery({ queryKey: ["annotations"], queryFn: listAnnotations });

  const counts: Partial<Record<ViewKey, number | undefined>> = {
    actors: actorsQuery.data?.length,
    cases: cases.length || undefined,
    annotations: annotationsQuery.data?.length,
  };

  function go(key: ViewKey, params?: Record<string, string>) {
    navigate(key, params);
    onNavigate?.();
  }

  return (
    <aside className="rail" aria-label="Main">
      <div className="rail-brand">
        <span className="rail-brand-mark">
          <BrandMark />
        </span>
        <span className="rail-brand-name">Chain Analysis</span>
      </div>

      <button type="button" className="rail-search" onClick={openSearch} title={`Search (${isMac ? "⌘" : "Ctrl+"}K)`}>
        <SearchIcon />
        <span>Search</span>
        <kbd>{isMac ? "⌘K" : "Ctrl K"}</kbd>
      </button>

      <nav className="rail-nav" aria-label="Pages">
        {NAV_ITEMS.map((item) => (
          <a
            key={item.key}
            href={`#${item.key}`}
            className="rail-item"
            aria-current={item.key === view ? "page" : undefined}
            title={collapsed ? item.label : undefined}
            onClick={(event) => {
              event.preventDefault();
              go(item.key);
            }}
          >
            <span className="rail-icon">{item.icon}</span>
            <span className="rail-label">{item.label}</span>
            {counts[item.key] !== undefined ? <span className="count">{counts[item.key]}</span> : null}
          </a>
        ))}
      </nav>

      <div className="rail-spacer" />

      <section className="rail-case" aria-label="Active case">
        {activeCase ? (
          <>
            <span className="rail-case-label">Working on</span>
            <a
              href={`#cases?case=${activeCase.id}`}
              className="rail-case-title"
              title={activeCase.title}
              onClick={(event) => {
                event.preventDefault();
                go("cases", { case: String(activeCase.id) });
              }}
            >
              {activeCase.title}
            </a>
            <span className="rail-case-meta">{pluralize(activeCase.item_count, "item")} so far</span>
            <div className="rail-case-actions">
              <button type="button" className="btn btn-sm" onClick={chooseCase}>
                Switch case
              </button>
            </div>
          </>
        ) : (
          <>
            <span className="rail-case-label">No active case</span>
            <span className="rail-case-meta">Pick one to collect findings from graphs and traces.</span>
            <div className="rail-case-actions">
              <button type="button" className="btn btn-sm" onClick={chooseCase}>
                Choose a case
              </button>
            </div>
          </>
        )}
      </section>

      {collapsed ? (
        <button
          type="button"
          className="btn btn-ghost btn-icon rail-case-icon"
          title={activeCase ? `Working on ${activeCase.title}` : "Choose a case"}
          aria-label={activeCase ? `Working on ${activeCase.title}` : "Choose a case"}
          onClick={() => (activeCase ? go("cases", { case: String(activeCase.id) }) : chooseCase())}
        >
          <CasesIcon />
        </button>
      ) : null}

      <div className="rail-footer">
        <HealthIndicator />
        <button
          type="button"
          className="btn btn-ghost btn-icon btn-sm"
          title={`${THEME_LABEL[theme]}. Click to change.`}
          aria-label={`${THEME_LABEL[theme]}. Click to change.`}
          onClick={() => setTheme(NEXT_THEME[theme])}
        >
          {theme === "dark" ? <MoonIcon /> : theme === "light" ? <SunIcon /> : <MonitorIcon />}
        </button>
        <button
          type="button"
          className="btn btn-ghost btn-icon btn-sm"
          title="Keyboard shortcuts (?)"
          aria-label="Keyboard shortcuts"
          onClick={openShortcuts}
        >
          <KeyboardIcon />
        </button>
        <button
          type="button"
          className="btn btn-ghost btn-icon btn-sm rail-collapse-toggle"
          title={collapsed ? "Expand navigation" : "Collapse navigation"}
          aria-label={collapsed ? "Expand navigation" : "Collapse navigation"}
          onClick={onToggleCollapsed}
        >
          {collapsed ? <ExpandIcon /> : <CollapseIcon />}
        </button>
      </div>
    </aside>
  );
}
