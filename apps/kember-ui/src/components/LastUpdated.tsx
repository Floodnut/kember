import { formatTimestamp } from "../lib/format";

export function LastUpdated({ value, refreshing }: { value: Date | null; refreshing: boolean }) {
  return (
    <p className="last-updated">
      {refreshing ? "Refreshing · " : "Updated · "}{formatTimestamp(value?.toISOString() ?? null)}
    </p>
  );
}
