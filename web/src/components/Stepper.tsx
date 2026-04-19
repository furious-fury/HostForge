type Step = {
  id: string | number;
  label: string;
};

type StepperProps = {
  steps: Step[];
  currentIndex: number;
  failedIndex?: number;
};

export function Stepper({ steps, currentIndex, failedIndex }: StepperProps) {
  return (
    <ol className="flex items-stretch border border-border bg-surface-alt">
      {steps.map((step, idx) => {
        const isFailed = failedIndex !== undefined && idx === failedIndex;
        const isCurrent = !isFailed && idx === currentIndex;
        const isDone = !isFailed && idx < currentIndex;
        let stateClass = "text-muted border-border";
        let dot = "○";
        if (isDone) {
          stateClass = "text-success border-success";
          dot = "●";
        } else if (isCurrent) {
          stateClass = "text-primary border-primary";
          dot = "●";
        } else if (isFailed) {
          stateClass = "text-danger border-danger";
          dot = "✕";
        }
        return (
          <li
            key={step.id}
            className={`flex flex-1 items-center gap-2 border-r last:border-r-0 px-4 py-3 ${stateClass}`}
          >
            <span className="mono text-sm">{dot}</span>
            <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] opacity-70">
              Step {idx + 1}
            </span>
            <span className="text-xs font-semibold uppercase tracking-wider">{step.label}</span>
          </li>
        );
      })}
    </ol>
  );
}
