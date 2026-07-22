import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DemoDataSource } from "../../../demo/source";
import { DashboardPage } from "../DashboardPage";

describe("DashboardPage", () => {
  it("keeps the approved header and exposes one complete dashboard", async () => {
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    expect(await screen.findByText("FlowLens")).toBeInTheDocument();
    expect(screen.getByText("采集正常")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "跟随系统" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "退出" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Overview" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "流量总览" })).toHaveClass(
      "page-title",
    );
    expect(
      screen.queryByText(
        "从当前速度到长期累计，把流量、连接、去向和数据质量放在一起看。",
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "全部" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "实时吞吐" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "实时目标分析" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "数据质量" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "存储健康" }),
    ).toBeInTheDocument();
    const topology = document.querySelector(".topology-panel");
    const confidence = document.querySelector(".confidence-panel");
    const targets = document.querySelector(".targets-panel");
    expect(topology).not.toBeNull();
    expect(confidence).not.toBeNull();
    expect(targets).not.toBeNull();
    expect(
      topology!.compareDocumentPosition(confidence!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
    expect(
      confidence!.compareDocumentPosition(targets!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
  });

  it("keeps realtime and historical modes visibly separate", async () => {
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    expect(
      await screen.findByRole("heading", { name: "实时吞吐" }),
    ).toBeInTheDocument();
    expect(screen.getByText("最近 60 分钟 · 1 秒采样")).toBeInTheDocument();
    expect(screen.getByText("可归因覆盖")).toBeInTheDocument();
    expect(
      screen.getByRole("progressbar", { name: "可归因覆盖" }),
    ).toHaveAttribute("aria-valuenow", "94.7");
    expect(document.querySelector(".quality-ring")).not.toBeInTheDocument();
    for (const label of [
      "1 分钟平均下载",
      "1 分钟平均上传",
      "5 分钟平均下载",
      "5 分钟平均上传",
      "60 分钟峰值下载",
      "60 分钟峰值上传",
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
    expect(screen.getByText(/10\.0 秒采样/)).toBeInTheDocument();
    expect(screen.getByText(/占全局 51\.6%/)).toBeInTheDocument();
    expect(screen.getByLabelText("第 1 名")).toHaveTextContent("1");
    expect(screen.getByLabelText("第 6 名")).toHaveTextContent("6");
    await userEvent.click(screen.getByRole("button", { name: "今天" }));
    expect(
      await screen.findByRole("heading", { name: "历史流量" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/SQLite 聚合/)).toBeInTheDocument();
    expect(screen.getByText("排行覆盖")).toBeInTheDocument();
    expect(screen.getByText("未归因流量")).toBeInTheDocument();
    expect(screen.getByText("Top K 之外")).toBeInTheDocument();
    for (const label of [
      "总流量",
      "较上一周期",
      "平均下载",
      "平均上传",
      "峰值下载",
      "峰值上传",
      "平均连接",
      "峰值连接",
      "数据完整率",
      "恢复流量",
      "重置次数",
      "质量事件",
      "边界估算",
      "最近运行",
    ]) {
      expect((await screen.findAllByText(label)).length).toBeGreaterThan(0);
    }
    expect(screen.getByText("精确边界")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "流量视图" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "速度视图" }),
    ).toBeInTheDocument();
    for (const dimension of [
      "目标 IP",
      "Endpoint",
      "端口",
      "TCP/UDP",
      "来源网段",
      "域名",
    ]) {
      expect(
        screen.getByRole("button", { name: dimension }),
      ).toBeInTheDocument();
    }
    expect(screen.getAllByText("sing-box 1.12.0")).toHaveLength(2);
    expect(screen.getByText("当前运行")).toBeInTheDocument();
    expect(
      screen.queryByText("最近 60 分钟 · 1 秒采样"),
    ).not.toBeInTheDocument();
  });

  it("marks custom-range boundary approximation explicitly", async () => {
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "自定义" }));
    await userEvent.click(screen.getByRole("button", { name: "应用" }));
    expect(await screen.findByText("已近似")).toBeInTheDocument();
  });

  it("uses topology only for target-like dimensions", async () => {
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "今天" }));
    expect(await screen.findByLabelText(/流量拓扑/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "端口" }));
    expect(await screen.findByLabelText("端口分布")).toBeInTheDocument();
    expect(screen.queryByLabelText(/流量拓扑/)).not.toBeInTheDocument();
    expect(screen.getAllByText("443").length).toBeGreaterThan(0);
  });

  it("exposes target and Demo read-only alias views on the same page", async () => {
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    expect(
      (await screen.findAllByText("Media API · 198.51.100.20:443")).length,
    ).toBeGreaterThanOrEqual(1);
    expect(
      await screen.findByRole("heading", { name: "实时目标分析" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/流量拓扑/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "管理别名" }));
    expect(
      screen.getByText("Demo 为只读，别名修改仅在生产模式提供。"),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "关闭别名" }));
    expect(screen.getByText("42.8 MiB")).toBeInTheDocument();
  });
});
