import { render } from "@testing-library/react";
import type { LiveTargetResponse } from "../../../api/contracts";
import { FlowMap } from "../FlowMap";

function targets(count: number): LiveTargetResponse[] {
  return Array.from({ length: count }, (_, index) => ({
    raw_endpoint: `192.0.2.${index + 1}:443`,
    display_name: `Service ${index + 1} · 192.0.2.${index + 1}:443`,
    network_code: 1,
    host: `192.0.2.${index + 1}`,
    upload_bytes_per_second: 0,
    download_bytes_per_second: 10,
  }));
}

describe("FlowMap", () => {
  it.each([
    [1, 107],
    [3, 221],
    [5, 335],
  ])("aligns residual branch after %i target rows", (count, y) => {
    const { container } = render(
      <FlowMap
        targets={targets(count)}
        globalRate={100}
        coverage={0.9}
        state="fresh"
      />,
    );
    expect(
      container.querySelector(".flow-rest-branch .flow-ribbon"),
    ).toHaveAttribute("d", `M415 188 C510 188 525 ${y} 625 ${y}`);
    expect(container.querySelectorAll(".flow-target-branch")).toHaveLength(
      count,
    );
  });

  it("pauses movement for stale and zero-rate branches", () => {
    const zero = targets(1);
    zero[0]!.download_bytes_per_second = 0;
    const { container, rerender } = render(
      <FlowMap targets={zero} globalRate={0} coverage={null} state="fresh" />,
    );
    expect(container.querySelectorAll(".flow-particles")).toHaveLength(0);
    rerender(
      <FlowMap
        targets={targets(1)}
        globalRate={100}
        coverage={0.9}
        state="stale"
      />,
    );
    expect(container.querySelectorAll(".flow-particles")).toHaveLength(0);
  });
});
