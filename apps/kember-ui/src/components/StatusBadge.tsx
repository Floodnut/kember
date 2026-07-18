const known = new Set(["Pending", "Running", "Succeeded", "Failed", "TimedOut", "Rejected", "Cancelled"]);

export function StatusBadge({ phase }: { phase: string | null }) {
  const label = phase ?? "Unknown";
  const tone = known.has(label) ? label.toLowerCase() : "unknown";
  return <span className={`status status-${tone}`}>{label}</span>;
}
