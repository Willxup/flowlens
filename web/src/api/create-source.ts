import { DemoDataSource } from "../demo/source";
import { buildMode, type BuildMode } from "./mode";
import { ProductionDataSource } from "./production";
import type { FlowLensDataSource } from "./source";

export function createDataSource(
  mode: BuildMode = buildMode(),
): FlowLensDataSource {
  return mode === "demo" ? new DemoDataSource() : new ProductionDataSource();
}
