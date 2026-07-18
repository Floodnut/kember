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
