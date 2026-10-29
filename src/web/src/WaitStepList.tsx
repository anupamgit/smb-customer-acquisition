import { WAIT_STEP_LABELS } from "./waitProgress";

type Props = {
  activeIndex: number;
};

export function WaitStepList({ activeIndex }: Props) {
  const allDone = activeIndex >= WAIT_STEP_LABELS.length;
  return (
    <ol className="steps" aria-label="Progress">
      {WAIT_STEP_LABELS.map((label, i) => {
        let stateClass = "is-pending";
        if (allDone || i < activeIndex) stateClass = "is-done";
        else if (i === activeIndex) stateClass = "is-active";
        return (
          <li
            key={label}
            className={`step ${stateClass}`}
            aria-current={!allDone && i === activeIndex ? "step" : undefined}
          >
            <span className="mark" aria-hidden="true">
              {!allDone && i === activeIndex ? <span className="mark-spin" /> : null}
            </span>
            <p className="step-label">{label}</p>
          </li>
        );
      })}
    </ol>
  );
}
