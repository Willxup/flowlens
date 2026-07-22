import { buildMode } from "../api/mode";

describe("buildMode", () => {
  it("accepts the two fixed build modes", () => {
    expect(buildMode("app")).toBe("app");
    expect(buildMode("demo")).toBe("demo");
  });

  it("rejects an unsupported mode", () => {
    expect(() => buildMode("preview")).toThrow(
      "FlowLens build mode is invalid",
    );
  });
});
