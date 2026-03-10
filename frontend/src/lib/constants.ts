export const DEFAULT_FLOW_TYPES = ["liquidity", "swaps", "bonds", "transfers"] as const;

export const DEFAULT_DISPLAY_MODE = "combined";

export function defaultActorGraphWindow(now = new Date()) {
  const end = new Date(now);
  const start = new Date(now);
  start.setDate(start.getDate() - 7);

  return {
    start,
    end,
  };
}
