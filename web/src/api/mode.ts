export type BuildMode = "app" | "demo";

export function buildMode(
  value = import.meta.env.VITE_FLOWLENS_MODE,
): BuildMode {
  if (value === "app" || value === "demo") return value;
  throw new Error("FlowLens build mode is invalid");
}
