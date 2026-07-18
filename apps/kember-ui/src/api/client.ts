import type {
  ApiErrorResponse,
  ItemsResponse,
  NamespaceView,
  TaskRunView,
  WorkerPoolView,
} from "./types";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
    this.name = "ApiError";
  }
}

export interface ApiClient {
  getNamespaces(signal: AbortSignal): Promise<ItemsResponse<NamespaceView>>;
  listWorkerPools(namespace: string, signal: AbortSignal): Promise<ItemsResponse<WorkerPoolView>>;
  getWorkerPool(namespace: string, name: string, signal: AbortSignal): Promise<WorkerPoolView>;
  listTaskRuns(namespace: string, signal: AbortSignal): Promise<ItemsResponse<TaskRunView>>;
  getTaskRun(namespace: string, name: string, signal: AbortSignal): Promise<TaskRunView>;
}

async function request<T>(path: string, signal: AbortSignal): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, { signal });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw new ApiError(0, "connection_error");
  }
  if (!response.ok) {
    const body = await response.json().catch(() => null) as ApiErrorResponse | null;
    throw new ApiError(response.status, body?.error.code ?? "connection_error");
  }
  return response.json() as Promise<T>;
}

const segment = (value: string) => encodeURIComponent(value);

export function createApiClient(): ApiClient {
  return {
    getNamespaces: (signal) => request("/api/v1/namespaces", signal),
    listWorkerPools: (namespace, signal) => request(
      `/api/v1/namespaces/${segment(namespace)}/worker-pools`,
      signal,
    ),
    getWorkerPool: (namespace, name, signal) => request(
      `/api/v1/namespaces/${segment(namespace)}/worker-pools/${segment(name)}`,
      signal,
    ),
    listTaskRuns: (namespace, signal) => request(
      `/api/v1/namespaces/${segment(namespace)}/task-runs`,
      signal,
    ),
    getTaskRun: (namespace, name, signal) => request(
      `/api/v1/namespaces/${segment(namespace)}/task-runs/${segment(name)}`,
      signal,
    ),
  };
}
