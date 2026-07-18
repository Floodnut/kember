import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { ApiProvider } from "./api/ApiContext";
import type { ApiClient } from "./api/client";

describe("App", () => {
  it("renders Kember navigation", () => {
    render(
      <MemoryRouter>
        <ApiProvider client={{
          getNamespaces: vi.fn().mockResolvedValue({ items: [{ cluster: "local", name: "default" }] }),
          listWorkerPools: vi.fn().mockResolvedValue({ items: [] }),
          getWorkerPool: vi.fn(),
          listTaskRuns: vi.fn().mockResolvedValue({ items: [] }),
          getTaskRun: vi.fn(),
        } satisfies ApiClient}>
          <App />
        </ApiProvider>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "Worker pools" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Task runs" })).toBeTruthy();
  });
});
