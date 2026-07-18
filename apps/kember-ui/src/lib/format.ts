export function formatDuration(seconds: number | null): string {
  if (seconds === null || !Number.isFinite(seconds) || seconds < 0) {
    return "—";
  }
  if (seconds < 1) {
    return `${Math.round(seconds * 1000)}ms`;
  }
  return `${seconds.toFixed(2).replace(/\.00$/, "")}s`;
}

export function formatTimestamp(value: string | null): string {
  if (value === null) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }
  return date.toISOString().replace("T", " ").replace(".000Z", " UTC");
}

export function formatActiveDuration(taskRun: {
  activeDurationSeconds: number | null;
  dispatchedAt: string | null;
  completedAt?: string | null;
}, nowMs = Date.now()): string {
  if (taskRun.activeDurationSeconds !== null) return formatDuration(taskRun.activeDurationSeconds);
  if (taskRun.dispatchedAt === null || taskRun.completedAt) return "—";
  const dispatchedMs = Date.parse(taskRun.dispatchedAt);
  if (!Number.isFinite(dispatchedMs) || nowMs < dispatchedMs) return "—";
  return `~${formatDuration((nowMs - dispatchedMs) / 1_000)}`;
}
