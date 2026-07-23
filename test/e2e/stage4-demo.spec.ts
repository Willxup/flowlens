import { expect, test } from "@playwright/test";

test("offline Demo exposes one rich responsive statistics dashboard", async ({
  page,
}) => {
  const businessRequests: string[] = [];
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith("/api/"))
      businessRequests.push(`${request.resourceType()}:${path}`);
  });

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("./");
  await expect(page.locator("main.app")).toHaveAttribute(
    "data-source-mode",
    "demo",
  );
  await expect(page.getByText("FlowLens")).toBeVisible();
  await expect(page.getByText("采集正常")).toBeVisible();
  await expect(page.locator(".live-status.ok i")).toHaveCSS(
    "animation-name",
    "status-pulse",
  );
  const liveStatusBox = await page.locator(".live-status").boundingBox();
  const themeToggleBox = await page.locator(".theme-toggle").boundingBox();
  expect(liveStatusBox).not.toBeNull();
  expect(themeToggleBox).not.toBeNull();
  expect(themeToggleBox!.x - (liveStatusBox!.x + liveStatusBox!.width)).toBeGreaterThanOrEqual(18);
  await expect(page.getByRole("button", { name: "跟随系统" })).toBeVisible();
  await expect(page.getByRole("button", { name: "退出" })).toBeVisible();
  await expect(page.getByRole("navigation")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "全部" })).toHaveCount(0);
  await expect(
    page.getByText(
      "从当前速度到长期累计，把流量、连接、去向和数据质量放在一起看。",
    ),
  ).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "流量总览" })).toHaveClass(
    /page-title/,
  );
  await expect(
    page.getByRole("heading", { name: "实时吞吐" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "实时目标分析" }),
  ).toBeVisible();
  await expect(page.locator(".targets-panel .target-item")).toHaveCount(6);
  await expect(page.getByText("94.7%")).toBeVisible();
  await expect(page.getByText("1 分钟平均下载")).toBeVisible();
  await expect(page.getByText("5 分钟平均上传")).toBeVisible();
  await expect(page.getByText("60 分钟峰值下载")).toBeVisible();
  const liveMetricBoxes = await page
    .locator(".metric-grid .chart-value")
    .evaluateAll((elements) =>
      elements.map((element) => {
        const box = element.getBoundingClientRect();
        return { x: box.x, y: box.y };
      }),
    );
  expect(liveMetricBoxes).toHaveLength(6);
  expect(
    liveMetricBoxes.every(
      (box) => Math.abs(box.y - liveMetricBoxes[0]!.y) < 2,
    ),
  ).toBe(true);
  await expect(page.getByText(/占全局 51\.6%/)).toBeVisible();
  const topologyBox = await page.locator(".topology-panel").boundingBox();
  const confidenceBox = await page.locator(".confidence-panel").boundingBox();
  const targetsBox = await page.locator(".targets-panel").boundingBox();
  expect(topologyBox).not.toBeNull();
  expect(confidenceBox).not.toBeNull();
  expect(targetsBox).not.toBeNull();
  expect(Math.abs(topologyBox!.y - confidenceBox!.y)).toBeLessThan(2);
  expect(Math.abs(topologyBox!.height - confidenceBox!.height)).toBeLessThan(2);
  expect(targetsBox!.y).toBeGreaterThan(
    Math.max(
      topologyBox!.y + topologyBox!.height,
      confidenceBox!.y + confidenceBox!.height,
    ),
  );
  const density = await page.evaluate(() => {
    const number = (selector: string, property: string) => {
      const element = document.querySelector(selector);
      if (!(element instanceof HTMLElement)) return null;
      return Number.parseFloat(getComputedStyle(element).getPropertyValue(property));
    };
    return {
      dashboardGap: number(".dashboard", "gap"),
      panelPadding: number(".hero-panel", "padding-top"),
      panelRadius: number(".hero-panel", "border-top-left-radius"),
      pageTitleSize: number(".page-title", "font-size"),
      panelTitleSize: number(".panel-head h2", "font-size"),
      summaryValueSize: number(".chart-summary .chart-value strong", "font-size"),
      chartHeight: document.querySelector(".chart-shell")?.getBoundingClientRect()
        .height,
      topologyHeight: document.querySelector(".topology")?.getBoundingClientRect()
        .height,
      targetHeight: document.querySelector(".target-item")?.getBoundingClientRect()
        .height,
    };
  });
  expect(density.dashboardGap).toBeLessThanOrEqual(14);
  expect(density.panelPadding).toBeLessThanOrEqual(22);
  expect(density.panelRadius).toBeLessThanOrEqual(24);
  expect(density.pageTitleSize).toBeLessThanOrEqual(36);
  expect(density.panelTitleSize).toBeLessThanOrEqual(18);
  expect(density.summaryValueSize).toBeLessThanOrEqual(30);
  expect(density.chartHeight).toBeLessThanOrEqual(260);
  expect(density.topologyHeight).toBeLessThanOrEqual(280);
  expect(density.targetHeight).toBeLessThanOrEqual(60);
  const firstTarget = await page.locator(".targets-panel .target-item").nth(0).boundingBox();
  const secondTarget = await page.locator(".targets-panel .target-item").nth(1).boundingBox();
  expect(firstTarget).not.toBeNull();
  expect(secondTarget).not.toBeNull();
  expect(Math.abs(firstTarget!.x - secondTarget!.x)).toBeLessThan(2);
  expect(secondTarget!.y).toBeGreaterThan(firstTarget!.y + firstTarget!.height);
  await expect(page.getByLabel("第 1 名")).toHaveText("1");
  await expect(page.getByLabel("第 6 名")).toHaveText("6");

  await page.getByRole("button", { name: "浅色模式" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.getByRole("button", { name: "深色模式" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await page.getByRole("button", { name: "今天" }).click();
  await expect(
    page.getByRole("heading", { name: "历史流量" }),
  ).toBeVisible();
  await expect(page.getByText("SQLite 聚合 · 自动选择分辨率")).toBeVisible();
  await expect(page.getByText("sing-box 1.12.0").first()).toBeVisible();
  await expect(page.getByText("当前运行")).toBeVisible();
  await expect(page.getByText("精确边界")).toBeVisible();
  const historyDashboardBox = await page.locator(".dashboard").boundingBox();
  const historyTopologyBox = await page
    .locator(".topology-panel")
    .boundingBox();
  const historyConfidenceBox = await page
    .locator(".confidence-panel")
    .boundingBox();
  const historyTargetsBox = await page
    .locator(".targets-panel")
    .boundingBox();
  expect(historyDashboardBox).not.toBeNull();
  expect(historyTopologyBox).not.toBeNull();
  expect(historyConfidenceBox).not.toBeNull();
  expect(historyTargetsBox).not.toBeNull();
  expect(
    Math.abs(historyTopologyBox!.width - historyDashboardBox!.width),
  ).toBeLessThan(2);
  expect(historyConfidenceBox!.y).toBeGreaterThan(
    historyTopologyBox!.y + historyTopologyBox!.height,
  );
  expect(
    Math.abs(historyConfidenceBox!.y - historyTargetsBox!.y),
  ).toBeLessThan(2);
  expect(
    Math.abs(historyConfidenceBox!.height - historyTargetsBox!.height),
  ).toBeLessThan(2);
  expect(historyTargetsBox!.width).toBeGreaterThan(historyConfidenceBox!.width);
  const todayTotal = await page
    .locator(".chart-summary .chart-value")
    .nth(2)
    .locator("strong")
    .textContent();
  expect(todayTotal).toBeTruthy();

  for (const dimension of [
    "目标 IP",
    "Endpoint",
    "端口",
    "TCP/UDP",
    "来源网段",
    "域名",
  ]) {
    await expect(page.getByRole("button", { name: dimension })).toBeVisible();
  }
  await expect(page.getByRole("button", { name: "速度视图" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(
    page.getByRole("img", { name: "历史平均上传和下载速度曲线" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "流量视图" }).click();
  await expect(
    page.getByRole("img", { name: "历史上传下载流量和累计曲线" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "速度视图" }).click();
  await page.getByRole("button", { name: "端口" }).click();
  await expect(page.getByLabel("端口分布")).toBeVisible();
  await expect(page.getByLabel(/流量拓扑/)).toHaveCount(0);
  await page.getByRole("button", { name: "Endpoint" }).click();
  await expect(page.getByLabel(/流量拓扑/)).toBeVisible();

  await page.getByRole("button", { name: "7 天" }).click();
  const sevenDayTotal = await page
    .locator(".chart-summary .chart-value")
    .nth(2)
    .locator("strong")
    .textContent();
  expect(sevenDayTotal).toBeTruthy();
  expect(sevenDayTotal).not.toBe(todayTotal);

  await page.getByRole("button", { name: "今年" }).click();
  const yearTotal = await page
    .locator(".chart-summary .chart-value")
    .nth(2)
    .locator("strong")
    .textContent();
  expect(yearTotal).not.toBe(sevenDayTotal);

  await page.getByRole("button", { name: "自定义" }).click();
  const customDialogBox = await page
    .getByRole("dialog", { name: "选择自定义日期" })
    .boundingBox();
  const customDateCardBox = await page
    .locator(".custom-date-card")
    .first()
    .boundingBox();
  const calendarDayBox = await page
    .getByRole("button", { name: "2026-07-16" })
    .boundingBox();
  expect(customDialogBox).not.toBeNull();
  expect(customDateCardBox).not.toBeNull();
  expect(calendarDayBox).not.toBeNull();
  expect(customDialogBox!.width).toBeLessThanOrEqual(430);
  expect(customDialogBox!.height).toBeLessThanOrEqual(480);
  expect(customDateCardBox!.height).toBeLessThanOrEqual(62);
  expect(calendarDayBox!.height).toBeLessThanOrEqual(30);
  await page.getByRole("button", { name: "2026-07-16" }).click();
  await page.getByRole("button", { name: "2026-07-18" }).click();
  await page.getByRole("button", { name: "应用" }).click();
  const customTotal = await page
    .locator(".chart-summary .chart-value")
    .nth(2)
    .locator("strong")
    .textContent();
  expect(customTotal).not.toBe(yearTotal);
  await expect(page.getByText("已近似")).toBeVisible();

  await page.getByRole("button", { name: "管理别名" }).click();
  await expect(
    page.getByText("Demo 为只读，别名修改仅在生产模式提供。"),
  ).toBeVisible();
  const desktopAliasInput = await page
    .locator(".alias-row input")
    .first()
    .boundingBox();
  const desktopAliasActions = await page
    .locator(".alias-row .alias-actions")
    .first()
    .boundingBox();
  const desktopClose = await page
    .getByRole("button", { name: "关闭别名" })
    .boundingBox();
  const desktopAliasRowCount = await page.locator(".alias-row").count();
  const desktopAliasActionCount = await page
    .locator(".alias-actions button")
    .count();
  expect(desktopAliasInput).not.toBeNull();
  expect(desktopAliasActions).not.toBeNull();
  expect(desktopClose).not.toBeNull();
  expect(
    Math.abs(
      desktopAliasInput!.y + desktopAliasInput!.height / 2 -
        (desktopAliasActions!.y + desktopAliasActions!.height / 2),
    ),
  ).toBeLessThan(2);
  expect(desktopClose!.width).toBeLessThanOrEqual(36);
  expect(desktopClose!.height).toBeLessThanOrEqual(36);
  expect(desktopAliasActionCount).toBe(desktopAliasRowCount);
  await page.getByRole("button", { name: "关闭别名" }).click();

  await page.setViewportSize({ width: 320, height: 800 });
  await expect(page.getByRole("heading", { name: "历史流量" })).toBeVisible();
  const mobileMinimumWidths = await page.evaluate(() => ({
    html: getComputedStyle(document.documentElement).minWidth,
    body: getComputedStyle(document.body).minWidth,
  }));
  expect(mobileMinimumWidths.html).not.toBe("320px");
  expect(mobileMinimumWidths.body).not.toBe("320px");
  const overflow = await page.evaluate(
    () =>
      document.documentElement.scrollWidth -
      document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(1);
  const mobileFlow = page.locator(".topology-mobile-flow");
  await expect(mobileFlow).toHaveCount(1);
  await expect(mobileFlow).toBeVisible();
  await expect(mobileFlow.locator(".mobile-target-path")).toHaveCount(3);
  await expect(page.locator(".topology-desktop-flow")).toHaveCSS(
    "display",
    "none",
  );
  const topology = await page.locator(".topology").boundingBox();
  const sourceOne = await page.locator(".node-source-one").boundingBox();
  const sourceTwo = await page.locator(".node-source-two").boundingBox();
  const gateway = await page.locator(".node-gateway").boundingBox();
  const firstTopologyTarget = await page
    .locator(".topology .node-target")
    .first()
    .boundingBox();
  expect(topology).not.toBeNull();
  expect(sourceOne).not.toBeNull();
  expect(sourceTwo).not.toBeNull();
  expect(gateway).not.toBeNull();
  expect(firstTopologyTarget).not.toBeNull();
  expect(Math.abs(sourceOne!.y - sourceTwo!.y)).toBeLessThan(2);
  expect(sourceTwo!.x).toBeGreaterThan(sourceOne!.x + sourceOne!.width);
  expect(gateway!.y).toBeGreaterThan(sourceOne!.y + sourceOne!.height);
  expect(Math.abs(gateway!.width - topology!.width)).toBeLessThan(2);
  expect(firstTopologyTarget!.y).toBeGreaterThan(
    gateway!.y + gateway!.height,
  );
  await expect(page.locator(".dashboard")).toHaveCSS(
    "grid-template-columns",
    /310px|300px|1fr/,
  );
  await page.getByRole("button", { name: "管理别名" }).click();
  const mobileSave = await page
    .locator(".alias-row")
    .first()
    .getByRole("button", { name: /保存$/ })
    .boundingBox();
  const mobileClose = await page
    .getByRole("button", { name: "关闭别名" })
    .boundingBox();
  expect(mobileSave).not.toBeNull();
  expect(mobileClose).not.toBeNull();
  expect(mobileSave!.width).toBeLessThanOrEqual(80);
  expect(mobileClose!.width).toBeLessThanOrEqual(36);
  expect(mobileClose!.height).toBeLessThanOrEqual(36);
  const dialogOverflow = await page.evaluate(
    () =>
      document.documentElement.scrollWidth -
      document.documentElement.clientWidth,
  );
  expect(dialogOverflow).toBeLessThanOrEqual(1);

  expect(pageErrors).toEqual([]);
  expect(businessRequests).toEqual([]);
});
