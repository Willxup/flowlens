import { useEffect, useState } from "react";
import type {
  BreakdownBy,
  BreakdownResponse,
  TimeSelection,
} from "../../api/contracts";
import { UnauthorizedError } from "../../api/production";
import type { FlowLensDataSource } from "../../api/source";
import { toHistoricalRange } from "../../lib/time-range";
import { buildTargetView } from "../targets/model";
import { buildHistoricalView } from "./model";

export function useHistoryViewModel(
  source: FlowLensDataSource,
  selection: TimeSelection,
  timezone: string,
  by: BreakdownBy,
  onUnauthorized: () => void,
  refreshKey = 0,
) {
  const [state, setState] = useState<{
    loading: boolean;
    error: boolean;
    view: ReturnType<typeof buildHistoricalView> | null;
    breakdown: BreakdownResponse | null;
  }>({
    loading: false,
    error: false,
    view: null,
    breakdown: null,
  });

  useEffect(() => {
    if (selection.kind === "live") return;
    const controller = new AbortController();
    let active = true;
    let loaded = false;
    const load = async () => {
      setState((current) =>
        loaded
          ? { ...current, loading: true, error: false }
          : { loading: true, error: false, view: null, breakdown: null },
      );
      try {
        const range = toHistoricalRange(selection, source.now(), timezone);
        const [overview, series, quality, breakdown] = await Promise.all([
          source.overview(range, controller.signal),
          source.series(range, controller.signal),
          source.quality(range, controller.signal),
          source.breakdown(range, by, controller.signal),
        ]);
        if (active) {
          loaded = true;
          setState({
            loading: false,
            error: false,
            view: buildHistoricalView(overview, series, quality),
            breakdown,
          });
        }
      } catch (error) {
        if (error instanceof UnauthorizedError) onUnauthorized();
        else if (active && !controller.signal.aborted)
          setState((current) => ({
            loading: false,
            error: true,
            view: loaded ? current.view : null,
            breakdown: loaded ? current.breakdown : null,
          }));
      }
    };
    void load();
    const interval =
      selection.kind === "preset" && selection.preset === "today"
        ? window.setInterval(load, 60_000)
        : undefined;
    return () => {
      active = false;
      controller.abort();
      if (interval !== undefined) window.clearInterval(interval);
    };
  }, [by, onUnauthorized, refreshKey, selection, source, timezone]);

  return {
    ...state,
    targets: state.breakdown === null ? null : buildTargetView(state.breakdown),
  };
}
