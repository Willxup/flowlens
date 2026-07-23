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
