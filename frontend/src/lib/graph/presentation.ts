import { type CytoscapeStyleBlock, type VisibleGraphNode } from "./types";

export function graphStylesheet(mode: "actor" | "explorer"): CytoscapeStyleBlock[] {
  const base: CytoscapeStyleBlock[] = [
    {
      selector: "node",
      style: {
        label: "",
        color: "#f5fbff",
        "background-color": "data(color)",
        "background-image": "data(chainLogo)",
        "background-fit": "contain",
        "background-width": "60%",
        "background-height": "60%",
        "background-opacity": 0.3,
        "background-clip": "node",
        width: 58,
        height: 58,
        "font-size": 11,
        "font-weight": 700,
        "text-wrap": "wrap",
        "text-max-width": 110,
        "text-valign": "center",
        "text-halign": "center",
        "border-width": 2,
        "border-color": "data(borderColor)",
        "overlay-padding": 8,
        "pie-size": "76%",
        "pie-1-background-color": "data(pie1Color)",
        "pie-1-background-size": "data(pie1Size)",
        "pie-2-background-color": "data(pie2Color)",
        "pie-2-background-size": "data(pie2Size)",
        "pie-3-background-color": "data(pie3Color)",
        "pie-3-background-size": "data(pie3Size)",
        "pie-4-background-color": "data(pie4Color)",
        "pie-4-background-size": "data(pie4Size)",
      },
    },
    {
      selector: "node:selected",
      style: {
        "border-width": 4,
        "border-color": "#ffdd44",
        "overlay-color": "rgba(255,221,68,0.15)",
        "overlay-opacity": 0.3,
      },
    },
    {
      selector: "node[kind = 'actor']",
      style: {
        shape: "round-rectangle",
        width: 136,
        height: 56,
        "font-size": 13,
        "background-color": "#0d1b2a",
        "background-width": "50%",
        "background-height": "80%",
        "background-image-opacity": 1,
        "background-opacity": 1,
        "border-width": 4,
        "pie-size": "0%",
      },
    },
    {
      selector: "node[kind = 'pool']",
      style: {
        shape: "diamond",
        width: 84,
        height: 84,
        "background-color": "#1e4f8f",
      },
    },
    {
      selector: "node[kind = 'actor_address']",
      style: {
        shape: "ellipse",
        width: 62,
        height: 62,
        "background-color": "#0d1b2a",
        "background-opacity": 1,
        "background-width": "60%",
        "background-height": "60%",
        "background-image-opacity": 1,
        "border-width": 4,
        "pie-size": "0%",
      },
    },
    {
      selector: "node[kind = 'external_address']",
      style: {
        shape: "ellipse",
        width: 62,
        height: 62,
      },
    },
    {
      selector: "node[kind = 'node']",
      style: {
        shape: "octagon",
        width: 94,
        height: 94,
        "background-color": "#c86b1f",
        "border-color": "#ffe0b8",
        "border-width": 4,
        color: "#fff7ea",
        "font-size": 12,
      },
    },
    {
      selector: "node[kind = 'contract_address']",
      style: {
        shape: "round-rectangle",
        width: 108,
        height: 60,
        "background-color": "#915a2b",
      },
    },
    {
      selector: "node[kind = 'bond_address']",
      style: {
        shape: "tag",
        "background-color": "#654590",
      },
    },
    {
      selector: "node[kind = 'inbound'], node[kind = 'router']",
      style: {
        shape: "round-rectangle",
        "background-color": "#176666",
      },
    },
    {
      selector: "node[kind = 'external_cluster']",
      style: {
        shape: "barrel",
        width: 120,
        height: 56,
        "background-color": "#164a47",
      },
    },
    {
      selector: "edge",
      style: {
        label: "data(edgeLabel)",
        width: "data(width)",
        "line-color": "data(lineColor)",
        "target-arrow-color": "data(lineColor)",
        "target-arrow-shape": "triangle",
        "curve-style": "bezier",
        "arrow-scale": 0.95,
        opacity: 0.82,
        color: "#d9ecff",
        "font-size": 9,
        "text-wrap": "wrap",
        "text-max-width": 190,
        "text-rotation": "autorotate",
        "text-background-color": "rgba(7, 16, 31, 0.88)",
        "text-background-opacity": 1,
        "text-background-shape": "roundrectangle",
        "text-background-padding": "3px",
        "text-events": "no",
        "text-margin-y": -8,
      },
    },
    {
      selector: "edge[actionClass = 'ownership']",
      style: {
        label: "",
        width: 1.4,
        "line-style": "dashed",
        "line-color": "#7a94bb",
        "target-arrow-color": "#7a94bb",
        opacity: 0.5,
      },
    },
  ];

  if (mode === "explorer") {
    base.splice(2, 0, {
      selector: "node[kind = 'explorer_target']",
      style: {
        shape: "hexagon",
        width: 110,
        height: 100,
        "background-color": "#e67e22",
        "border-color": "#f5c76e",
        "border-width": 4,
        "font-size": 13,
      },
    });
  }

  return base;
}

export function graphLayoutNodeSize(mode: "actor" | "explorer", node: VisibleGraphNode) {
  if (mode === "explorer" && node.kind === "explorer_target") {
    return { width: 120, height: 108 };
  }
  if (node.kind === "actor") {
    return { width: 150, height: 64 };
  }
  if (node.kind === "pool") {
    return { width: 96, height: 96 };
  }
  return { width: 84, height: 72 };
}

export function graphLineColor(actionClass: string) {
  switch (actionClass) {
    case "liquidity":
      return "#78d6c4";
    case "swaps":
      return "#52a8ff";
    case "bonds":
      return "#d9a6ff";
    case "ownership":
      return "#7a94bb";
    default:
      return "#cfdcff";
  }
}

export function edgeWidth(usdSpot: number) {
  if (!usdSpot || usdSpot <= 0) {
    return 2.2;
  }
  return Math.min(12, 2 + Math.log10(usdSpot + 1) * 1.6);
}
