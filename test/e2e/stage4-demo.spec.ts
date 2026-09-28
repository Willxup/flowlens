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
