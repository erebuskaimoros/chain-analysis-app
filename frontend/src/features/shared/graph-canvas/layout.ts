import cytoscape from "cytoscape";
import { graphLayoutNodeSize, type VisibleGraphEdge, type VisibleGraphNode } from "../../../lib/graph";

interface ElkLayoutEngine {
  layout(graph: object): Promise<{ children?: Array<{ id: string; x?: number; y?: number }> }>;
}

let elkEnginePromise: Promise<ElkLayoutEngine> | null = null;

// ELK layouts on large graphs are CPU-heavy; run them in a web worker when the
// environment provides one (browsers) and fall back to the bundled main-thread
// engine otherwise (jsdom tests).
function getElkEngine(): Promise<ElkLayoutEngine> {
  if (!elkEnginePromise) {
    elkEnginePromise = createElkEngine();
  }
  return elkEnginePromise;
}

async function createElkEngine(): Promise<ElkLayoutEngine> {
  if (typeof Worker !== "undefined") {
    try {
      const [{ default: ELK }, { default: ElkWorker }] = await Promise.all([
        import("elkjs/lib/elk-api.js"),
        import("elkjs/lib/elk-worker.min.js?worker"),
      ]);
      return new ELK({
        workerFactory: () => new ElkWorker(),
      }) as unknown as ElkLayoutEngine;
    } catch {
      // fall through to the bundled engine
    }
  }
  const { default: ELK } = await import("elkjs/lib/elk.bundled.js");
  return new ELK() as unknown as ElkLayoutEngine;
}

export async function applyElkLayout(
  cy: cytoscape.Core,
  mode: "actor" | "explorer",
  nodes: VisibleGraphNode[],
  preservedPositions: Map<string, cytoscape.Position> = new Map(),
  isCancelled: () => boolean = () => false
) {
  const graph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.layered.spacing.nodeNodeBetweenLayers": "110",
      "elk.spacing.nodeNode": "42",
      "elk.padding": "[top=32,left=32,bottom=32,right=32]",
    },
    children: nodes.map((node) => {
      const size = graphLayoutNodeSize(mode, node);
      return {
        id: node.id,
        width: size.width,
        height: size.height,
      };
    }),
    edges: cy
      .edges()
      .toArray()
      .map((edge) => ({
        id: edge.id(),
        sources: [String(edge.data("source"))],
        targets: [String(edge.data("target"))],
      })),
  };

  const elk = await getElkEngine();
  const result = await elk.layout(graph);
  // A newer sync (expansion, reset) may have superseded this layout while ELK
  // was running; applying it now would stomp fresher positions.
  if (isCancelled()) {
    return;
  }
  const positions = new Map(
    (result.children || []).map((child: { id: string; x?: number; y?: number }) => [
      child.id,
      { x: child.x || 0, y: child.y || 0 },
    ])
  );

  const positionMap: Record<string, { x: number; y: number }> = {};
  nodes.forEach((node) => {
    const preserved = preservedPositions.get(node.id);
    positionMap[node.id] = preserved || positions.get(node.id) || { x: Number(node.depth || 0) * 180, y: 80 };
  });

  cy.layout({
    name: "preset",
    fit: false,
    animate: false,
    positions: positionMap,
  }).run();
}

const ANCHOR_SPACING_X = 180;
const ANCHOR_MIN_GAP = 130;
const ANCHOR_ROW_GAP = 150;

// Positions freshly added nodes next to their already-positioned neighbors so an
// expansion grows out of the node it came from instead of re-laying-out the
// whole graph (which would destroy the user's mental map). Nodes without any
// positioned neighbor are stacked in a row below the existing drawing.
export function placeNewNodesNearAnchors(
  cy: cytoscape.Core,
  mode: "actor" | "explorer",
  nodes: VisibleGraphNode[],
  edges: VisibleGraphEdge[],
  newNodeIDs: string[]
) {
  const newIDSet = new Set(newNodeIDs);
  const placed = new Map<string, cytoscape.Position>();
  cy.nodes().forEach((element) => {
    if (!newIDSet.has(element.id()) && typeof element.position === "function") {
      placed.set(element.id(), element.position());
    }
  });

  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  const pending = [...newNodeIDs];

  function anchorTargets(id: string) {
    const targets: cytoscape.Position[] = [];
    for (const edge of edges) {
      if (edge.target === id) {
        const source = placed.get(edge.source);
        if (source) {
          targets.push({ x: source.x + ANCHOR_SPACING_X, y: source.y });
        }
      }
      if (edge.source === id) {
        const target = placed.get(edge.target);
        if (target) {
          targets.push({ x: target.x - ANCHOR_SPACING_X, y: target.y });
        }
      }
    }
    return targets;
  }

  function minGapFor(id: string) {
    const node = nodeByID.get(id);
    if (!node) {
      return ANCHOR_MIN_GAP;
    }
    const size = graphLayoutNodeSize(mode, node);
    return Math.max(ANCHOR_MIN_GAP, Math.max(size.width, size.height) + 40);
  }

  function place(id: string, base: cytoscape.Position) {
    const position = resolveCollision(base, placed, minGapFor(id));
    const element = cy.getElementById(id) as cytoscape.NodeSingular;
    if (element.nonempty() && typeof element.position === "function") {
      element.position(position);
    }
    placed.set(id, position);
  }

  // Repeated passes let chains of new nodes anchor to each other in order.
  let progress = true;
  while (pending.length && progress) {
    progress = false;
    for (let index = 0; index < pending.length; ) {
      const id = pending[index];
      const targets = anchorTargets(id);
      if (!targets.length) {
        index += 1;
        continue;
      }
      const base = {
        x: targets.reduce((sum, point) => sum + point.x, 0) / targets.length,
        y: targets.reduce((sum, point) => sum + point.y, 0) / targets.length,
      };
      place(id, base);
      pending.splice(index, 1);
      progress = true;
    }
  }

  if (!pending.length) {
    return;
  }

  // Disconnected leftovers: a row under the current drawing.
  let minX = 0;
  let maxY = 0;
  if (placed.size) {
    minX = Number.POSITIVE_INFINITY;
    maxY = Number.NEGATIVE_INFINITY;
    placed.forEach((position) => {
      minX = Math.min(minX, position.x);
      maxY = Math.max(maxY, position.y);
    });
  }
  let x = minX;
  const y = maxY + ANCHOR_ROW_GAP;
  pending.forEach((id) => {
    place(id, { x, y });
    x += ANCHOR_SPACING_X;
  });
}

function resolveCollision(
  base: cytoscape.Position,
  placed: Map<string, cytoscape.Position>,
  minGap: number
): cytoscape.Position {
  if (isFree(base, placed, minGap)) {
    return { ...base };
  }
  for (let ring = 1; ring <= 12; ring += 1) {
    const radius = ring * minGap * 0.9;
    const steps = 6 * ring;
    for (let step = 0; step < steps; step += 1) {
      const angle = (step / steps) * Math.PI * 2;
      const candidate = {
        x: base.x + Math.cos(angle) * radius,
        y: base.y + Math.sin(angle) * radius,
      };
      if (isFree(candidate, placed, minGap)) {
        return candidate;
      }
    }
  }
  return { ...base };
}

function isFree(candidate: cytoscape.Position, placed: Map<string, cytoscape.Position>, minGap: number) {
  for (const position of placed.values()) {
    if (Math.hypot(position.x - candidate.x, position.y - candidate.y) < minGap) {
      return false;
    }
  }
  return true;
}
