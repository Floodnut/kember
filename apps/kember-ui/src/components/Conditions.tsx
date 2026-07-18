import type { ConditionView } from "../api/types";
import { formatTimestamp } from "../lib/format";

export function Conditions({ conditions }: { conditions: ConditionView[] }) {
  if (conditions.length === 0) return <p className="muted">No conditions</p>;
  return (
    <div className="conditions">
      {conditions.map((condition, index) => (
        <article key={`${condition.type}-${index}`}>
          <div><strong>{condition.type}</strong> · {condition.status}</div>
          <div>{condition.reason ?? "—"}</div>
          {condition.message && <p>{condition.message}</p>}
          <time>{formatTimestamp(condition.lastTransitionTime)}</time>
        </article>
      ))}
    </div>
  );
}
