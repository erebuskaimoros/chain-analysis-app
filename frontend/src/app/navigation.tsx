import type { ReactNode } from "react";
import { ActorsIcon, CasesIcon, ExplorerIcon, GraphIcon, HomeIcon, TagIcon, TraceIcon } from "./icons";
import type { ViewKey } from "./router";

export interface NavItem {
  key: ViewKey;
  label: string;
  icon: ReactNode;
  // Analysis pages give the map the whole width, so the rail starts collapsed.
  workspace?: boolean;
}

export const NAV_ITEMS: NavItem[] = [
  { key: "home", label: "Home", icon: <HomeIcon /> },
  { key: "graph", label: "Actor graph", icon: <GraphIcon />, workspace: true },
  { key: "explorer", label: "Explorer", icon: <ExplorerIcon />, workspace: true },
  { key: "trace", label: "Trace", icon: <TraceIcon /> },
  { key: "actors", label: "Actors", icon: <ActorsIcon /> },
  { key: "cases", label: "Cases", icon: <CasesIcon /> },
  { key: "annotations", label: "Labels", icon: <TagIcon /> },
];
