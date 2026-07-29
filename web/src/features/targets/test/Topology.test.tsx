import { render } from "@testing-library/react";
import type { ByteString } from "../../../api/contracts";
import { Topology } from "../Topology";
import type { HistoricalTargetRow } from "../TargetList";

function targets(count: number): HistoricalTargetRow[] {
  return Array.from({ length: count }, (_, index) => ({
    rawValue: `192.0.2.${index + 1}:443`,
    displayName: `目标 ${index + 1}`,
    networkCode: 1,
    totalBytes: "3" as ByteString,
    uploadBytes: "1" as ByteString,
    downloadBytes: "2" as ByteString,
  }));
}

it("draws the mobile target trunk only as far as the available targets", () => {
  const { container, rerender } = render(<Topology targets={[]} />);
  expect(container.querySelector(".mobile-target-trunk")).toBeNull();

  rerender(<Topology targets={targets(1)} />);
  expect(container.querySelector(".mobile-target-trunk")).toHaveAttribute(
    "d",
    "M150 120V128H14V162.5",
  );

  rerender(<Topology targets={targets(2)} />);
  expect(container.querySelector(".mobile-target-trunk")).toHaveAttribute(
    "d",
    "M150 120V128H14V212.5",
  );
});

it("stretches the desktop flow paths across wide historical panels", () => {
  const { container } = render(<Topology targets={targets(3)} />);

  const flow = container.querySelector(".topology-desktop-flow");
  expect(flow).toHaveAttribute("preserveAspectRatio", "none");
  expect(flow?.querySelector(".flow-path")?.getAttribute("d")).toMatch(/^M0 /);
  expect(
    Array.from(flow?.querySelectorAll(".flow-path.target") ?? []).map((path) =>
      path.getAttribute("d"),
    ),
  ).toEqual([
    "M450 150 C600 150 640 50 900 50",
    "M450 150 C600 150 640 150 900 150",
    "M450 150 C600 150 640 250 900 250",
  ]);
});
