import { useEffect, useState } from "react";
import type { SeriesResponse } from "../../api/contracts";
import { UnauthorizedError } from "../../api/production";
import type { FlowLensDataSource } from "../../api/source";

export function useTimelineHistory(
  source: FlowLensDataSource,
  onUnauthorized: () => void,
) {
  const [data, setData] = useState<SeriesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let pending = false;
    async function load() {
      if (pending) return;
      pending = true;
      setLoading(true);
      try {
        const value = await source.series(
          { kind: "preset", preset: "24h" },
          controller.signal,
        );
        if (!controller.signal.aborted) {
          setData(value);
          setError(false);
        }
      } catch (error) {
        if (controller.signal.aborted) return;
        if (error instanceof UnauthorizedError) onUnauthorized();
        else setError(true);
      } finally {
        pending = false;
        if (!controller.signal.aborted) setLoading(false);
      }
    }
    void load();
    const timer = window.setInterval(() => void load(), 60000);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [source, onUnauthorized, revision]);
  return {
    data,
    loading,
    error,
    retry: () => setRevision((value) => value + 1),
  };
}
