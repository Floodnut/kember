import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Kember API client", () => {
  it("encodes resource path segments and returns the response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ name: "scan/a" })));
    vi.stubGlobal("fetch", fetchMock);
    const signal = new AbortController().signal;

    const response = await createApiClient().getTaskRun("team-a", "scan/a", signal);

    expect(response.name).toBe("scan/a");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/namespaces/team-a/task-runs/scan%2Fa",
      { signal },
    );
  });

  it("maps the stable API error envelope", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ error: { code: "resource_not_found", message: "missing" } }),
      { status: 404, headers: { "Content-Type": "application/json" } },
    )));

    const promise = createApiClient().getTaskRun("team-a", "missing", new AbortController().signal);

    await expect(promise).rejects.toEqual(new ApiError(404, "resource_not_found"));
  });
});
