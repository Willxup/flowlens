import { expect, test } from "@playwright/test";

test("mobile login preserves keyboard access and clears rejected credentials", async ({
  page,
}) => {
  const errors: string[] = [];
  const submissions: unknown[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.setViewportSize({ width: 320, height: 720 });
  await page.route("**/api/v1/session", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(new URL(route.request().url()).search).toBe("");
    submissions.push(route.request().postDataJSON());
    await route.fulfill({
      status: 401,
      json: { error: "fixture private diagnostic" },
    });
  });
  await page.goto("http://127.0.0.1:4175/login");
  const key = page.getByLabel("共享访问密钥");
  const submit = page.getByRole("button", {
    name: "进入 FlowLens",
    exact: true,
  });
  await expect(key).toHaveAttribute("type", "password");
  await expect(submit).toBeDisabled();
  for (const theme of ["浅色模式", "深色模式"]) {
    await page.getByRole("button", { name: theme }).click();
    await expect(
      page.getByRole("heading", { name: "进入 FlowLens", exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () =>
          document.documentElement.scrollWidth -
          document.documentElement.clientWidth,
      ),
    ).toBeLessThanOrEqual(1);
  }
  await key.fill("bad-fixture");
  await key.press("Enter");
  await expect(page.getByRole("alert")).toHaveText(
    "无法登录，请检查密钥后重试。",
  );
  await expect(key).toHaveValue("");
  await expect(submit).toBeDisabled();
  await expect(page.getByText("fixture private diagnostic")).toHaveCount(0);
  expect(submissions).toEqual([{ access_key: "bad-fixture" }]);
  expect(errors).toEqual([]);
});
