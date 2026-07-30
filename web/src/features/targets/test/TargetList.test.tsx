import { render, screen, within } from "@testing-library/react";
import { asByteString } from "../../../lib/format";
import { TargetList } from "../TargetList";

describe("TargetList", () => {
  it("shows only the alias as the live title and keeps the raw endpoint below", () => {
    render(
      <TargetList
        live={[
          {
            raw_endpoint: "10.34.44.5:34422",
            display_name: "Office gateway · 10.34.44.5:34422",
            network_code: 1,
            host: "10.34.44.5",
            download_bytes_per_second: 43830477,
            upload_bytes_per_second: 2694211174,
          },
        ]}
        liveTotalRate={2738041651}
      />,
    );

    const item = screen.getByText("Office gateway").closest(".target-item");
    expect(item).not.toBeNull();
    expect(
      within(item as HTMLElement).getAllByText("10.34.44.5:34422"),
    ).toHaveLength(1);
    expect(
      within(item as HTMLElement).queryByText(
        "Office gateway · 10.34.44.5:34422",
      ),
    ).not.toBeInTheDocument();
  });

  it("does not repeat an unaliased endpoint in historical details", () => {
    render(
      <TargetList
        historical={[
          {
            rawValue: "10.34.44.5:34422",
            displayName: "10.34.44.5:34422",
            networkCode: 1,
            totalBytes: asByteString("2738041651"),
            downloadBytes: asByteString("43830477"),
            uploadBytes: asByteString("2694211174"),
          },
        ]}
      />,
    );

    const item = screen.getByText("10.34.44.5:34422").closest(".target-item");
    expect(item).not.toBeNull();
    expect(
      within(item as HTMLElement).getByLabelText("下载 41.8 MiB"),
    ).toHaveTextContent("↓ 41.8 MiB");
    expect(
      within(item as HTMLElement).getByLabelText("上传 2.5 GiB"),
    ).toHaveTextContent("↑ 2.5 GiB");
    expect(within(item as HTMLElement).getByText("TCP")).toBeInTheDocument();
    expect(
      within(item as HTMLElement).getAllByText("10.34.44.5:34422"),
    ).toHaveLength(1);
    expect(
      within(item as HTMLElement).queryByText(
        "10.34.44.5:34422 · TCP · ↓ 41.8 MiB · ↑ 2.5 GiB",
      ),
    ).not.toBeInTheDocument();
  });
});
