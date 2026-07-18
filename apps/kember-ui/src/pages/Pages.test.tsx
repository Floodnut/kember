import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import type { ApiClient } from "../api/client";
import { ApiError } from "../api/client";
import { ApiProvider } from "../api/ApiContext";
import type { TaskRunView, WorkerPoolView } from "../api/types";

const pool: WorkerPoolView = {
  cluster: "local",
  namespace: "team-a",
  name: "scanner",
  generation: 1,
  executionMode: "exec",
  lifecycleProfile: "warmLease",
  capacity: { desired: 2, starting: 0, ready: 1, leased: 1, terminating: 0 },
  conditions: [{
    type: "Ready",
    status: "True",
    reason: "CapacityReady",
    message: "one worker ready",
    lastTransitionTime: "2026-07-18T01:00:00Z",
  }],
};

const task: TaskRunView = {
  cluster: "local",
  namespace: "team-a",
  name: "scan-source",
  createdAt: "2026-07-18T01:00:00Z",
  workerPool: "scanner",
  phase: "Paused",
  assignedWorker: "scanner-abc",
  dispatchedAt: "2026-07-18T01:00:01Z",
  completedAt: null,
  conditions: [],
  queueWaitSeconds: 1,
  activeDurationSeconds: null,
};

function fakeApi(overrides: Partial<ApiClient> = {}): ApiClient {
  return {
    getNamespaces: vi.fn().mockResolvedValue({ items: [{ cluster: "local", name: "team-a" }] }),
    listWorkerPools: vi.fn().mockResolvedValue({ items: [pool] }),
    getWorkerPool: vi.fn().mockResolvedValue(pool),
    listTaskRuns: vi.fn().mockResolvedValue({ items: [task] }),
    getTaskRun: vi.fn().mockResolvedValue(task),
    ...overrides,
  };
}

function renderRoute(path: string, client: ApiClient = fakeApi()) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ApiProvider client={client}>
        <App />
      </ApiProvider>
    </MemoryRouter>,
  );
}

describe("lifecycle pages", () => {
  it("renders the WorkerPool operations table", async () => {
    renderRoute("/worker-pools");

    expect(await screen.findByRole("link", { name: "scanner" })).toBeTruthy();
    expect(screen.getByText("warmLease")).toBeTruthy();
    expect(screen.getByText("local / team-a")).toBeTruthy();
  });

  it("shows only TaskRuns related to a WorkerPool", async () => {
    const unrelated = { ...task, name: "other-task", workerPool: "other" };
    renderRoute("/worker-pools/scanner", fakeApi({
      listTaskRuns: vi.fn().mockResolvedValue({ items: [task, unrelated] }),
    }));

    expect(await screen.findByText("CapacityReady")).toBeTruthy();
    expect(screen.getByRole("link", { name: "scan-source" })).toBeTruthy();
    expect(screen.queryByText("other-task")).toBeNull();
  });

  it("preserves an unknown TaskRun phase in the list", async () => {
    renderRoute("/task-runs");

    expect(await screen.findByRole("link", { name: "scan-source" })).toBeTruthy();
    expect(screen.getByText("Paused")).toBeTruthy();
  });

  it("renders the TaskRun timeline without inventing completion", async () => {
    renderRoute("/task-runs/scan-source");

    expect(await screen.findByRole("heading", { name: "scan-source" })).toBeTruthy();
    expect(screen.getByText("Created")).toBeTruthy();
    expect(screen.getByText("Dispatched")).toBeTruthy();
    expect(screen.getByText("Completed")).toBeTruthy();
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
  });

  it("distinguishes an empty list from an initial API error", async () => {
    const { unmount } = renderRoute("/worker-pools", fakeApi({
      listWorkerPools: vi.fn().mockResolvedValue({ items: [] }),
    }));
    expect(await screen.findByText("No worker pools")).toBeTruthy();
    unmount();

    renderRoute("/worker-pools", fakeApi({
      listWorkerPools: vi.fn().mockRejectedValue(new ApiError(503, "kubernetes_api_unavailable")),
    }));
    expect(await screen.findByText("Kubernetes API is unavailable")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });
});
