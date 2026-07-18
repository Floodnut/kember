import { formatTimestamp } from "../lib/format";

export function Timeline({ createdAt, dispatchedAt, completedAt }: {
  createdAt: string | null;
  dispatchedAt: string | null;
  completedAt: string | null;
}) {
  const steps: Array<{ label: string; value: string | null }> = [
    { label: "Created", value: createdAt },
    { label: "Dispatched", value: dispatchedAt },
    { label: "Completed", value: completedAt },
  ];
  return (
    <ol className="timeline" aria-label="TaskRun lifecycle">
      {steps.map(({ label, value }) => (
        <li key={label} className={value === null ? "timeline-pending" : "timeline-complete"}>
          <strong>{label}</strong>
          <time>{formatTimestamp(value)}</time>
        </li>
      ))}
    </ol>
  );
}
