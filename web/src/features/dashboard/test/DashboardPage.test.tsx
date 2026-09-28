import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type {
  HistoricalSelection,
  LiveTargetsResponse,
  OverviewResponse,
} from "../../../api/contracts";
import { UnauthorizedError } from "../../../api/production";
import { DemoDataSource } from "../../../demo/source";
import { DashboardPage } from "../DashboardPage";

vi.mock("../../traffic/TrafficChart", () => ({
  TrafficChart: ({
    mode,
    historyView,
  }: {
    mode: "live" | "history";
    historyView?: "traffic" | "speed";
  }) => (
    <div
      role="img"
      aria-label={
        mode === "live"
          ? "实时上传和下载速度曲线"
          : historyView === "speed"
            ? "历史平均上传和下载速度曲线"
            : "历史上传下载流量和累计曲线"
      }
    />
  ),
}));

class FailingHistorySource extends DemoDataSource {
  override async overview(
    _range: HistoricalSelection,
  ): Promise<OverviewResponse> {
    throw new Error("fixture unavailable");
  }
}
class DifferentWindowLiveSource extends DemoDataSource {
  override async liveTargets(): Promise<LiveTargetsResponse> {
    const value = await super.liveTargets();
    return {
      ...value,
      global_upload_bytes_per_second: 2_900_000,
      global_download_bytes_per_second: 15_340_000,
    };
  }
}
class FailingTargetsSource extends DemoDataSource {
  override async liveTargets(): Promise<LiveTargetsResponse> {
    throw new Error("fixture unavailable");
  }
}
class FailingLogoutSource extends DemoDataSource {
  override async logout(): Promise<void> {
    throw new Error("fixture unavailable");
  }
}
class UnauthorizedLogoutSource extends DemoDataSource {
  override async logout(): Promise<void> {
    throw new UnauthorizedError();
  }
}
class NoAuthSource extends DemoDataSource {
  override readonly demo = false;
  override async status() {
    return { ...(await super.status()), auth_enabled: false };
  }
}

function navigation() {
  return within(screen.getByRole("navigation", { name: "工作区" }));
}

describe("DashboardPage", () => {
  beforeAll(() => {
    Object.defineProperty(window, "scrollTo", {
      configurable: true,
      value: vi.fn(),
    });
  });
  it("starts with the live signal and navigates four separate workspaces", async () => {
    const user = userEvent.setup();
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    expect(
      screen.getByRole("link", { name: "FlowLens GitHub 仓库" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "实时总览" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "当前吞吐" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: "实时上传和下载速度曲线" }),
    ).toBeInTheDocument();
    expect(await screen.findByText(/目标快照正常/)).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "流向构成" }),
    ).toBeInTheDocument();
    expect(document.querySelectorAll(".flow-ribbon").length).toBeGreaterThan(0);
    expect(
      navigation().getByRole("button", { name: /实时总览/ }),
    ).toHaveAttribute("aria-current", "page");
    await user.click(navigation().getByRole("button", { name: /目标探索/ }));
    expect(
      screen.getByRole("heading", { name: "目标探索" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "当前吞吐" }),
    ).not.toBeInTheDocument();
    await user.click(navigation().getByRole("button", { name: /历史分析/ }));
    expect(
      screen.getByRole("heading", { name: "历史分析" }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("heading", { name: "历史流量" }),
    ).toBeInTheDocument();
    await user.click(navigation().getByRole("button", { name: /质量与存储/ }));
    expect(
      screen.getByRole("heading", { name: "质量与存储" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "存储健康" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Top K 之外")).toBeInTheDocument();
    expect(screen.getAllByText("sing-box 1.12.0")).toHaveLength(2);
  });

  it("searches targets and keeps aliases available", async () => {
    const user = userEvent.setup();
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    await user.click(navigation().getByRole("button", { name: /目标探索/ }));
    expect(await screen.findAllByLabelText("第 1 名")).toHaveLength(1);
    expect(document.querySelectorAll(".target-item")).toHaveLength(6);
    await user.type(
      screen.getByRole("searchbox", { name: "搜索目标" }),
      "Media API",
    );
    expect(document.querySelectorAll(".target-item")).toHaveLength(1);
    expect(screen.getByText("Media API")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "管理别名" }));
    expect(
      screen.getByText("Demo 为只读，别名修改仅在生产模式提供。"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "关闭别名" }));
  });

  it("keeps historical dimensions, ranges and both chart modes", async () => {
    const user = userEvent.setup();
    render(
      <DashboardPage source={new DemoDataSource()} onUnauthorized={vi.fn()} />,
    );
    await user.click(navigation().getByRole("button", { name: /历史分析/ }));
    expect(
      await screen.findByRole("img", { name: "历史平均上传和下载速度曲线" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "流量视图" }));
    expect(
      screen.getByRole("img", { name: "历史上传下载流量和累计曲线" }),
    ).toBeInTheDocument();
    for (const name of [
      "目标 IP",
      "Endpoint",
      "端口",
      "TCP/UDP",
      "来源网段",
      "域名",
    ]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
    await user.click(screen.getByRole("button", { name: "端口" }));
    expect(await screen.findAllByText("443")).not.toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "全部" }));
    expect(screen.getByText("不适用")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "自定义" }));
    await user.click(screen.getByRole("button", { name: "2026-07-16" }));
    await user.click(screen.getByRole("button", { name: "2026-07-18" }));
    await user.click(screen.getByRole("button", { name: "应用" }));
    expect(await screen.findByText("已近似")).toBeInTheDocument();
  });

  it("uses target snapshot global rate for approximate shares", async () => {
    const user = userEvent.setup();
    render(
      <DashboardPage
        source={new DifferentWindowLiveSource()}
        onUnauthorized={vi.fn()}
      />,
    );
    await user.click(navigation().getByRole("button", { name: /目标探索/ }));
    expect(await screen.findByText(/占全局 25\.8%/)).toBeInTheDocument();
  });

  it("distinguishes unavailable target and history data", async () => {
    const user = userEvent.setup();
    const view = render(
      <DashboardPage
        source={new FailingTargetsSource()}
        onUnauthorized={vi.fn()}
      />,
    );
    await user.click(navigation().getByRole("button", { name: /目标探索/ }));
    expect(
      await screen.findByText("目标数据暂时无法加载。"),
    ).toBeInTheDocument();
    view.unmount();
    render(
      <DashboardPage
        source={new FailingHistorySource()}
        onUnauthorized={vi.fn()}
      />,
    );
    await user.click(navigation().getByRole("button", { name: /历史分析/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("历史查询失败");
  });

  it("respects disabled authentication and handles logout failure", async () => {
    const noAuth = render(
      <DashboardPage source={new NoAuthSource()} onUnauthorized={vi.fn()} />,
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "退出" }),
      ).not.toBeInTheDocument(),
    );
    noAuth.unmount();
    const onUnauthorized = vi.fn();
    const failed = render(
      <DashboardPage
        source={new FailingLogoutSource()}
        onUnauthorized={onUnauthorized}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "退出" }));
    expect(onUnauthorized).not.toHaveBeenCalled();
    expect(
      await screen.findByRole("button", { name: "退出失败，请重试" }),
    ).toBeInTheDocument();
    failed.unmount();
    render(
      <DashboardPage
        source={new UnauthorizedLogoutSource()}
        onUnauthorized={onUnauthorized}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "退出" }));
    expect(onUnauthorized).toHaveBeenCalledOnce();
  });
});
