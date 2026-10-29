import { useEffect, useState } from "react";
import {
  isStepVisuallyDone,
  serverCompletedStepCount,
  waitStepActiveIndex,
} from "./waitProgress";

type RunLike = { id: string; status: string; stage?: string } | null;

/**
 * Each step stays active at least MIN_STEP_VISIBLE_MS once it becomes current.
 * A step may only finish (checkmark) after the server has completed that phase.
 * After success, results wait until step 3 has met its minimum too.
 */
export function useWaitStepDisplay(run: RunLike): { activeIndex: number; holdingResults: boolean } {
  const [activeSince, setActiveSince] = useState<(number | null)[]>([null, null, null]);
  const [tick, setTick] = useState(0);

  const inWaitFlow = run?.status === "running" || run?.status === "succeeded";
  const completedCount = run ? serverCompletedStepCount(run.status, run.stage) : 0;

  useEffect(() => {
    setActiveSince([null, null, null]);
  }, [run?.id]);

  useEffect(() => {
    if (!inWaitFlow) {
      return;
    }
    setActiveSince((prev) => {
      const next = [...prev];
      if (next[0] === null) {
        next[0] = Date.now();
      }
      return next;
    });
  }, [inWaitFlow, run?.id]);

  useEffect(() => {
    if (!inWaitFlow) {
      return;
    }
    const now = Date.now();
    setActiveSince((prev) => {
      const next = [...prev];
      let changed = false;
      for (let i = 1; i < 3; i++) {
        if (next[i] !== null) {
          continue;
        }
        if (isStepVisuallyDone(completedCount, next[i - 1] ?? null, i - 1, now)) {
          next[i] = now;
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [inWaitFlow, completedCount, tick, run?.id]);

  useEffect(() => {
    if (!inWaitFlow) {
      return;
    }
    const id = setInterval(() => setTick((n) => n + 1), 200);
    return () => clearInterval(id);
  }, [inWaitFlow, run?.id]);

  void tick;
  const now = Date.now();
  const activeIndex = inWaitFlow ? waitStepActiveIndex(completedCount, activeSince, now) : 0;
  const holdingResults = run?.status === "succeeded" && activeIndex < 3;

  return { activeIndex, holdingResults };
}
