import { expect, test } from "@playwright/test";

test("motion stays interruptible and restores scroll without blocking controls", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 800 });
  await page.goto("./");
  const nav = page.getByRole("navigation", { name: "工作区", exact: true });
  await expect(page.locator(".side-nav .selection-indicator")).toBeVisible();
  await page.evaluate(() => window.scrollTo(0, 280));
  await nav.getByRole("button", { name: /目标探索/ }).click();
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
  await nav.getByRole("button", { name: /实时总览/ }).click();
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(280);
  // Consecutive inputs must end at the newest selection.
  await nav.getByRole("button", { name: /目标探索/ }).click();
  await nav.getByRole("button", { name: /历史分析/ }).click();
  await nav.getByRole("button", { name: /目标探索/ }).click();
  await expect(nav.getByRole("button", { name: /目标探索/ })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expect
    .poll(() =>
      page.evaluate(() => {
        const indicator = document
          .querySelector(".side-nav .selection-indicator")!
          .getBoundingClientRect();
        const button = document
          .querySelector('.side-nav [aria-current="page"]')!
          .getBoundingClientRect();
        return Math.abs(indicator.top - button.top);
      }),
    )
    .toBeLessThan(1);
  await page.getByRole("button", { name: "按名称", exact: true }).click();
  const names = await page.locator(".target-main strong").allTextContents();
  await page.getByRole("button", { name: "按流量", exact: true }).click();
  await page.getByRole("searchbox", { name: "搜索目标" }).fill("Media API");
  await expect(page.locator(".target-item")).toHaveCount(1);
  await page.getByRole("searchbox", { name: "搜索目标" }).fill("");
  await expect(page.locator(".target-item")).toHaveCount(6);
  expect(names.length).toBeGreaterThan(0);

  const opener = page.getByRole("button", { name: "管理别名" });
  await opener.click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(opener).toBeFocused();
  await opener.click();
  await page.getByRole("button", { name: "关闭别名", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeHidden();

  await page.emulateMedia({ reducedMotion: "reduce" });
  await nav.getByRole("button", { name: /历史分析/ }).click();
  await page.getByRole("button", { name: "自定义", exact: true }).click();
  expect(
    await page
      .locator(".custom-range-dialog")
      .evaluate((el) => parseFloat(getComputedStyle(el).animationDuration)),
  ).toBeLessThan(0.001);
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
  await page.setViewportSize({ width: 390, height: 844 });
  const mobile = page.getByRole("navigation", { name: "移动工作区" });
  await mobile.getByRole("button", { name: "目标探索", exact: true }).click();
  await expect(page.locator(".mobile-nav .selection-indicator")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
