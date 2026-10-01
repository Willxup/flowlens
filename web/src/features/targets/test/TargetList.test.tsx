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

  it("sorts and scales large historical byte totals without precision loss", () => {
    const { container } = render(
      <TargetList
        historical={[
          {
            rawValue: "smaller",
            displayName: "Smaller",
            networkCode: 1,
            totalBytes: asByteString("2199023255552"),
            downloadBytes: asByteString("2199023255552"),
            uploadBytes: asByteString("0"),
          },
          {
            rawValue: "larger",
            displayName: "Larger",
            networkCode: 1,
            totalBytes: asByteString("3298534883328"),
            downloadBytes: asByteString("3298534883328"),
            uploadBytes: asByteString("0"),
          },
        ]}
        historicalGlobalBytes={asByteString("5497558138880")}
      />,
    );
    const rows = container.querySelectorAll(".target-item");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Larger");
    expect(rows[1]).toHaveTextContent("Smaller");
    expect(rows[0]?.querySelector(".target-bar i")).toHaveStyle({
      width: "100%",
    });
    expect(rows[1]?.querySelector(".target-bar i")).toHaveStyle({
      width: "66.6%",
    });
  });

  it("distinguishes unsupported dimensions, no traffic and fetch errors", () => {
    const { rerender } = render(
      <TargetList historical={[]} available={false} />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("不支持这个维度");
    rerender(<TargetList historical={[]} noTraffic />);
    expect(screen.getByRole("status")).toHaveTextContent("没有流量");
    rerender(<TargetList historical={[]} error />);
    expect(screen.getByRole("status")).toHaveTextContent("无法加载");
  });

  it("distinguishes zero global rate from a missing denominator", () => {
    const row = {
      raw_endpoint: "192.0.2.1:443",
      display_name: "Service",
      network_code: 1,
      host: "192.0.2.1",
      download_bytes_per_second: 0,
      upload_bytes_per_second: 0,
    };
    const { rerender } = render(<TargetList live={[row]} liveTotalRate={0} />);
    expect(screen.getByText("全局速率为 0")).toBeInTheDocument();
    rerender(<TargetList live={[row]} liveTotalRate={null} />);
    expect(screen.getByText("占比未知")).toBeInTheDocument();
  });
});
