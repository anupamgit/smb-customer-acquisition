import { describe, expect, it } from "vitest";
import {
  activeWaitStepIndex,
  isStepVisuallyDone,
  MIN_STEP_VISIBLE_MS,
  productWaitLine,
  serverCompletedStepCount,
  waitStepActiveIndex,
} from "./waitProgress";

describe("activeWaitStepIndex", () => {
  it("maps ingest to step 1", () => {
    expect(activeWaitStepIndex("ingest_normalize")).toBe(0);
    expect(activeWaitStepIndex("build_signals")).toBe(0);
  });
  it("maps discover to step 2", () => {
    expect(activeWaitStepIndex("discover_candidates")).toBe(1);
  });
  it("maps llm phases to step 3", () => {
    expect(activeWaitStepIndex("evaluate_opportunities")).toBe(2);
    expect(activeWaitStepIndex("explain")).toBe(2);
  });
});

describe("serverCompletedStepCount", () => {
  it("counts finished steps from active index while running", () => {
    expect(serverCompletedStepCount("running", "discover_candidates")).toBe(1);
    expect(serverCompletedStepCount("running", "evaluate_opportunities")).toBe(2);
  });
  it("is three on success", () => {
    expect(serverCompletedStepCount("succeeded", "complete")).toBe(3);
  });
});

describe("step dwell", () => {
  const t0 = 1_000_000;

  it("requires server done and minimum time on step", () => {
    expect(isStepVisuallyDone(1, t0, 0, t0 + MIN_STEP_VISIBLE_MS - 1)).toBe(false);
    expect(isStepVisuallyDone(1, t0, 0, t0 + MIN_STEP_VISIBLE_MS)).toBe(true);
    expect(isStepVisuallyDone(0, t0, 0, t0 + MIN_STEP_VISIBLE_MS)).toBe(false);
  });

  it("holds step 3 before all complete", () => {
    const since = [t0, t0 + 1000, t0 + 2000];
    expect(waitStepActiveIndex(3, since, t0 + 2500)).toBe(2);
    expect(waitStepActiveIndex(3, since, t0 + 3000)).toBe(3);
  });
});

describe("productWaitLine", () => {
  it("needs three products", () => {
    expect(productWaitLine(["Coffee", "Latte"])).toBeNull();
    expect(productWaitLine(["Coffee", "Latte", "Muffin"])).toContain("coffee");
  });
});
