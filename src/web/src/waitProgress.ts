/** Owner-facing wait steps. Stage IDs are server-only; never show them in the UI. */

export const WAIT_STEP_LABELS = [
  "Reviewing your sales patterns",
  "Looking for nearby partner businesses",
  "Choosing the best partners for you",
] as const;

/** Minimum time each step stays active on screen once it becomes the current step. */
export const MIN_STEP_VISIBLE_MS = 1000;

/** Active step index (0–2) from the run's current server stage. */
export function activeWaitStepIndex(stage: string | undefined): number {
  switch (stage) {
    case "evaluate_opportunities":
    case "rank_actions":
    case "explain":
      return 2;
    case "discover_candidates":
      return 1;
    case "complete":
      return 3;
    default:
      return 0;
  }
}

/** How many owner-facing steps the server has finished (0–3). */
export function serverCompletedStepCount(status: string, stage: string | undefined): number {
  if (status === "succeeded") {
    return 3;
  }
  if (status !== "running") {
    return 0;
  }
  return activeWaitStepIndex(stage);
}

export function isStepVisuallyDone(
  completedCount: number,
  activeSince: number | null,
  stepIndex: number,
  now: number,
): boolean {
  if (completedCount <= stepIndex) {
    return false;
  }
  if (activeSince === null) {
    return false;
  }
  return now - activeSince >= MIN_STEP_VISIBLE_MS;
}

/** Index of the active step (0–2), or 3 when all steps are visually complete. */
export function waitStepActiveIndex(
  completedCount: number,
  activeSince: (number | null)[],
  now: number,
): number {
  for (let i = 0; i < WAIT_STEP_LABELS.length; i++) {
    if (!isStepVisuallyDone(completedCount, activeSince[i] ?? null, i, now)) {
      return i;
    }
  }
  return WAIT_STEP_LABELS.length;
}

export function productWaitLine(products: string[]): string | null {
  if (products.length < 3) return null;
  const a = products[0].toLowerCase();
  const b = products[1].toLowerCase();
  const c = products[2].toLowerCase();
  return `Looking at what your regulars often buy—${a}, ${b}, and ${c}.`;
}
