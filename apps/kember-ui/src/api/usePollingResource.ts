import { useCallback, useEffect, useRef, useState } from "react";

export interface PollingResource<T> {
  data: T | undefined;
  loading: boolean;
  refreshing: boolean;
  error: unknown | null;
  updatedAt: Date | null;
  retry: () => void;
}

export function usePollingResource<T>(
  loader: (signal: AbortSignal) => Promise<T>,
  intervalMs = 5_000,
): PollingResource<T> {
  const [data, setData] = useState<T>();
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown | null>(null);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const hasDataRef = useRef(false);
  const triggerRef = useRef<() => void>(() => undefined);

  useEffect(() => {
    let mounted = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let active: AbortController | undefined;

    const load = async () => {
      if (active !== undefined) return;
      active = new AbortController();
      setLoading(!hasDataRef.current);
      setRefreshing(hasDataRef.current);
      try {
        const next = await loader(active.signal);
        if (!mounted) return;
        hasDataRef.current = true;
        setData(next);
        setError(null);
        setUpdatedAt(new Date());
      } catch (nextError) {
        if (!mounted || (nextError instanceof DOMException && nextError.name === "AbortError")) return;
        setError(nextError);
      } finally {
        active = undefined;
        if (mounted) {
          setLoading(false);
          setRefreshing(false);
          timer = setTimeout(load, intervalMs);
        }
      }
    };

    triggerRef.current = () => {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
      void load();
    };
    void load();

    return () => {
      mounted = false;
      if (timer !== undefined) clearTimeout(timer);
      active?.abort();
    };
  }, [intervalMs, loader]);

  const retry = useCallback(() => {
    setError(null);
    triggerRef.current();
  }, []);

  return { data, loading, refreshing, error, updatedAt, retry };
}
