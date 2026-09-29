import { mkdirSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test } from "@playwright/test";

const artifacts = resolve(process.cwd(), "../.flowlens-dev/artifacts");

test("offline observatory works across workspaces, themes and viewports", async ({
  page,
}) => {
  const businessRequests: string[] = [];
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.startsWith("/api/"))
      businessRequests.push(request.url());
  });
  mkdirSync(artifacts, { recursive: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("./");
  await expect(page.locator(".app-shell")).toHaveAttribute(
    "data-source-mode",
    "demo",
  );
  await expect(page.getByRole("heading", { name: "当前吞吐" })).toBeVisible();
  await expect(page.locator(".flow-target-branch")).toHaveCount(5);
  await expect(page.locator(".flow-rest-branch")).toHaveCount(1);
  await expect(page.getByText("可归因覆盖 94.7%")).toBeVisible();
  await expect(page.locator(".chart-shell svg")).toBeVisible();
  await expect(
    page.getByText("正在加载 24 小时历史，实时样本仍正常显示。"),
  ).toHaveCount(0);
  await expect(page.locator(".navigator-download")).not.toHaveAttribute(
    "d",
    "",
  );
  await expect(page.getByText("跟随实时 · 最近 60 分钟")).toBeVisible();
  await expect(page.locator(".brand-mark")).toHaveAttribute(
    "src",
    "./favicon.svg",
  );
  const shellGeometry = await page.evaluate(() => {
    const box = (selector: string) =>
      document.querySelector(selector)!.getBoundingClientRect();
    return {
      brandBottom: box(".brand").bottom,
      headerBottom: box(".topbar").bottom,
      railBottom: box(".sidebar-rail").bottom,
      workspaceBottom: box(".workspace-shell").bottom,
    };
  });
  expect(
    Math.abs(shellGeometry.brandBottom - shellGeometry.headerBottom),
  ).toBeLessThanOrEqual(1);
  expect(
    Math.abs(shellGeometry.railBottom - shellGeometry.workspaceBottom),
  ).toBeLessThanOrEqual(1);
  await page.evaluate(() =>
    window.scrollTo(0, document.documentElement.scrollHeight),
  );
  expect(
    await page
      .locator(".sidebar")
      .evaluate((element) => Math.abs(element.getBoundingClientRect().top)),
  ).toBeLessThanOrEqual(1);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.getByRole("button", { name: "深色模式" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.locator(".page-title").click();
  await page.mouse.move(0, 0);
  await page.screenshot({
    path: resolve(artifacts, "observatory-dark.png"),
    fullPage: true,
    animations: "disabled",
  });
  await page.getByRole("button", { name: "浅色模式" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.locator(".page-title").click();
  await page.mouse.move(0, 0);
  await page.screenshot({
    path: resolve(artifacts, "observatory-light.png"),
    fullPage: true,
    animations: "disabled",
  });

  await page.getByRole("button", { name: "查看 24 小时" }).click();
  await expect(page.getByText("浏览历史 · 已暂停跟随")).toBeVisible();
  await expect(page.locator(".timeline-range")).toContainText(
    "秒级样本 + 历史聚合",
  );
  await page.getByRole("slider", { name: "预览结束时间" }).press("ArrowLeft");
  await expect(page.getByText("浏览历史 · 已暂停跟随")).toBeVisible();
  await page.getByRole("button", { name: "回到实时" }).click();
  const navigator = await page.locator(".timeline-navigator svg").boundingBox();
  if (!navigator) throw new Error("missing timeline navigator");
  await page.mouse.move(
    navigator.x + navigator.width * 0.98,
    navigator.y + navigator.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    navigator.x + navigator.width * 0.5,
    navigator.y + navigator.height / 2,
    { steps: 8 },
  );
  await page.mouse.up();
  await expect(page.getByText("浏览历史 · 已暂停跟随")).toBeVisible();
  await expect(page.locator(".timeline-range")).toContainText(
    "历史聚合 · 非秒级",
  );
  await page.getByRole("button", { name: "回到实时" }).click();
  await expect(page.locator(".timeline-range")).toContainText("秒级样本");

  const side = page.getByRole("navigation", { name: "工作区" });
  await side.getByRole("button", { name: /目标探索/ }).click();
  await expect(page.getByRole("heading", { name: "目标探索" })).toBeVisible();
  await expect(page.locator(".target-item")).toHaveCount(6);
  await page.getByRole("searchbox", { name: "搜索目标" }).fill("Media API");
  await expect(page.locator(".target-item")).toHaveCount(1);
  await expect(page.getByText("占全局 51.6%")).toBeVisible();
  await page.getByRole("button", { name: "管理别名" }).click();
  await expect(
    page.getByText("Demo 为只读，别名修改仅在生产模式提供。"),
  ).toBeVisible();
  await page.getByRole("button", { name: "关闭别名" }).click();

  await side.getByRole("button", { name: /历史分析/ }).click();
  await expect(page.getByRole("heading", { name: "历史流量" })).toBeVisible();
  await expect(
    page.getByRole("img", { name: "历史平均上传和下载速度曲线" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "流量视图" }).click();
  await expect(
    page.getByRole("img", { name: "历史上传下载流量和累计曲线" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "7 天" }).click();
  await expect(page.getByRole("button", { name: "7 天" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  for (const dimension of [
    "目标 IP",
    "Endpoint",
    "端口",
    "TCP/UDP",
    "来源网段",
    "域名",
  ]) {
    await expect(
      page.getByRole("button", { name: dimension, exact: true }),
    ).toBeVisible();
  }
  await page.getByRole("button", { name: "端口", exact: true }).click();
  await expect(page.locator(".target-item")).not.toHaveCount(0);
  await page.getByRole("button", { name: "全部", exact: true }).click();
  await expect(page.getByText("不适用")).toBeVisible();
  await page.getByRole("button", { name: "自定义" }).click();
  await page.getByRole("button", { name: "2026-07-16" }).click();
  await page.getByRole("button", { name: "2026-07-18" }).click();
  await page.getByRole("button", { name: "应用" }).click();
  await expect(page.getByText("已近似")).toBeVisible();

  await side.getByRole("button", { name: /质量与存储/ }).click();
  await expect(page.getByRole("heading", { name: "存储健康" })).toBeVisible();
  await expect(page.getByText("Top K 之外")).toBeVisible();
  await expect(page.getByText("sing-box 1.12.0").first()).toBeVisible();
  await page.setViewportSize({ width: 320, height: 760 });
  for (const label of ["实时总览", "目标探索", "历史分析", "质量与存储"]) {
    await expect(
      page
        .getByRole("navigation", { name: "移动工作区" })
        .getByRole("button", { name: label }),
    ).toBeVisible();
  }
  await page
    .getByRole("navigation", { name: "移动工作区" })
    .getByRole("button", { name: "实时总览" })
    .click();
  await expect(page.getByRole("heading", { name: "当前吞吐" })).toBeVisible();
  await expect(page.locator(".flow-network")).toBeHidden();
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    ),
  ).toBeLessThanOrEqual(1);
  await page.locator(".page-title").click();
  await page.screenshot({
    path: resolve(artifacts, "observatory-mobile.png"),
    fullPage: true,
    animations: "disabled",
  });
  await page.emulateMedia({ reducedMotion: "reduce" });
  const duration = await page
    .locator(".signal-dot")
    .evaluate((element) => getComputedStyle(element).animationDuration);
  expect(duration).toBe("1e-05s");
  expect(pageErrors).toEqual([]);
  expect(businessRequests).toEqual([]);
});
