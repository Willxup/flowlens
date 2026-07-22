import { useEffect, useMemo, useState } from "react";
import type {
  LiveSampleResponse,
  LiveTargetsResponse,
  StatusResponse,
} from "../../api/contracts";
import { UnauthorizedError } from "../../api/production";
import type { FlowLensDataSource } from "../../api/source";
import { buildLiveView } from "./model";

export function useLiveViewModel(
  source: FlowLensDataSource,
  status: StatusResponse,
  enabled: boolean,
  onStatus: (level: StatusResponse["status"], reason: string) => void,
  onUnauthorized: () => void,
) {
  const [samples, setSamples] = useState<LiveSampleResponse[]>([]);
  const [targets, setTargets] = useState<LiveTargetsResponse | null>(null);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    const loadTargets = async () => {
      try {
        const value = await source.liveTargets();
        if (active) setTargets(value);
      } catch (error) {
        if (error instanceof UnauthorizedError) onUnauthorized();
      }
    };
    void loadTargets();
    const interval = window.setInterval(loadTargets, 10_000);
    const unsubscribe = source.subscribeLive((event) => {
      if (!active) return;
      if (event.type === "snapshot") setSamples(event.samples);
      if (event.type === "sample")
        setSamples((current) => [...current, event.sample].slice(-3600));
      if (event.type === "status") onStatus(event.status, event.reason);
    }, setConnected);
    return () => {
      active = false;
      window.clearInterval(interval);
      unsubscribe();
      setConnected(false);
    };
  }, [enabled, onStatus, onUnauthorized, source]);

  return useMemo(
    () => buildLiveView(samples, status, targets, connected),
    [connected, samples, status, targets],
  );
}
