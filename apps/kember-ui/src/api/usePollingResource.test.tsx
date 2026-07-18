import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePollingResource } from "./usePollingResource";

afterEach(() => {
  vi.useRealTimers();
});

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

describe("usePollingResource", () => {
  it("keeps successful data when a refresh fails and supports retry", async () => {
    vi.useFakeTimers();
    const loader = vi.fn()
      .mockResolvedValueOnce("first")
      .mockRejectedValueOnce(new Error("refresh failed"))
      .mockResolvedValueOnce("second");
    const { result } = renderHook(() => usePollingResource(loader, 5_000));

    expect(result.current.loading).toBe(true);
    await act(flush);
    expect(result.current.data).toBe("first");

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await flush();
    });
    expect(result.current.data).toBe("first");
    expect(result.current.error).toBeTruthy();

    await act(async () => {
      result.current.retry();
      await flush();
    });
    expect(result.current.data).toBe("second");
    expect(result.current.error).toBeNull();
  });

  it("does not overlap requests and aborts the active request on unmount", async () => {
    vi.useFakeTimers();
    let signal: AbortSignal | undefined;
    const loader = vi.fn((currentSignal: AbortSignal) => {
      signal = currentSignal;
      return new Promise<string>(() => undefined);
    });
    const { unmount } = renderHook(() => usePollingResource(loader, 5_000));

    await act(flush);
    vi.advanceTimersByTime(15_000);
    expect(loader).toHaveBeenCalledTimes(1);

    unmount();
    expect(signal?.aborted).toBe(true);
  });
});
