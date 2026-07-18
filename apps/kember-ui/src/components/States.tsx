import { ApiError } from "../api/client";

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "kubernetes_api_unavailable") return "Kubernetes API is unavailable";
    if (error.code === "namespace_not_allowed") return "Namespace is not available";
    if (error.code === "resource_not_found") return "Resource was not found";
  }
  return "Kember API is unavailable";
}

export function Loading() {
  return <p className="state">Loading…</p>;
}

export function Empty({ children }: { children: string }) {
  return <p className="state">{children}</p>;
}

export function InitialError({ error, retry }: { error: unknown; retry: () => void }) {
  return (
    <div className="state state-error" role="alert">
      <p>{errorMessage(error)}</p>
      <button type="button" onClick={retry}>Retry</button>
    </div>
  );
}

export function RefreshError() {
  return <p className="refresh-error" role="alert">Refresh failed. Showing the last successful response.</p>;
}
