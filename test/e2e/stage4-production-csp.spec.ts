import { expect, test } from "@playwright/test";
const csp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self'",
  "img-src 'self' data:",
  "connect-src 'self'",
  "font-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

test("production bundle works with the shipped CSP and named SSE events", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 720 });
  const cspErrors: string[] = [];
  const apiRequests: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/"))
      apiRequests.push(`${url.pathname}${url.search}`);
  });
  page.on("console", (message) => {
    if (/content security policy|refused to apply inline/i.test(message.text()))
      cspErrors.push(message.text());
  });
  await page.route("http://127.0.0.1:4175/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/") {
      const response = await route.fetch();
      await route.fulfill({
        response,
        headers: {
          ...response.headers(),
          "Content-Security-Policy": csp,
        },
      });
      return;
    }
    if (path === "/api/v1/live") {
      await route.fulfill({
        contentType: "text/event-stream",
        body: [
          "id: 1",
          "event: snapshot",
          'data: {"sequence":1,"samples":[{"timestamp":1700000000,"upload_bytes_per_second":12,"download_bytes_per_second":34,"active_connections":2,"status":"ok"}]}',
          "",
          "id: 2",
          "event: status",
          'data: {"sequence":2,"status":"ok","reason":"ready","ready":true}',
          "",
        ].join("\n"),
      });
      return;
    }
    if (path === "/api/v1/session" && route.request().method() === "DELETE") {
      await route.fulfill({ status: 503 });
      return;
    }
    const response = fixtureResponse(path);
    if (response !== undefined) {
      await route.fulfill({ json: response });
      return;
    }
    await route.continue();
  });

  await page.goto("http://127.0.0.1:4175/");
  await expect(page.locator("main.app")).toHaveAttribute(
    "data-source-mode",
    "app",
  );
  await expect(page.getByRole("heading", { name: "实时吞吐" })).toBeVisible();
  await expect.poll(() => apiRequests).toContain("/api/v1/connections/live");
  await expect(
    page.getByText("Fixture · 198.51.100.20:443").first(),
  ).toBeVisible();
  await expect(page.locator(".chart-shell svg")).toBeVisible();
  await page.getByRole("button", { name: "30 天" }).click();
  await expect(page.getByRole("heading", { name: "历史流量" })).toBeVisible();
  await expect.poll(() => apiRequests).toContain("/api/v1/overview?range=30d");
  await expect
    .poll(() => apiRequests)
    .toContain("/api/v1/series?range=30d&resolution=auto");
  await expect
    .poll(() => apiRequests)
    .toContain("/api/v1/breakdown?range=30d&by=endpoint");
  expect(apiRequests.some((request) => /[?&](from|to)=\d/.test(request))).toBe(
    false,
  );
  await expect(page.locator(".topology-desktop-flow")).toHaveAttribute(
    "preserveAspectRatio",
    "none",
  );
  await page.setViewportSize({ width: 1280, height: 900 });
  await expect(page.locator(".topology-desktop-flow")).toBeVisible();
  const [flowBox, sourceBox, targetBox] = await Promise.all([
    page.locator(".topology-desktop-flow").boundingBox(),
    page.locator(".node-source-one").boundingBox(),
    page.locator(".node-target-0").boundingBox(),
  ]);
  if (flowBox === null || sourceBox === null || targetBox === null)
    throw new Error("missing topology boxes");
  expect(sourceBox.x + sourceBox.width - flowBox.x).toBeCloseTo(4, 0);
  expect(flowBox.x + flowBox.width - targetBox.x).toBeCloseTo(4, 0);
  await page.setViewportSize({ width: 320, height: 720 });
  await page.locator(".logout-button").click();
  await expect(
    page.getByRole("button", { name: "退出失败，请重试" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    ),
  ).toBeLessThanOrEqual(1);
  expect(cspErrors).toEqual([]);
});

function fixtureResponse(path: string): unknown {
  if (path === "/api/v1/status")
    return {
      status: "ok",
      reason: "ready",
      timezone: "UTC",
      auth_enabled: true,
      capabilities: {
        connection_id: true,
        source: true,
        destination: true,
        port: true,
        network: true,
        domain: true,
      },
    };
  if (path === "/api/v1/storage")
    return {
      database_bytes: "4096",
      wal_bytes: "0",
      soft_limit_bytes: "1048576",
      protecting: false,
      last_rollup_cleanup: null,
    };
  if (path === "/api/v1/labels") return { labels: [] };
  if (path === "/api/v1/label-candidates") return { candidates: [] };
  if (path === "/api/v1/connections/live")
    return {
      observed_at: 1_700_000_000,
      interval_millis: 1000,
      active_connections: 2,
      global_upload_bytes_per_second: 12,
      global_download_bytes_per_second: 34,
      connection_coverage: 1,
      targets: [
        {
          raw_endpoint: "198.51.100.20:443",
          display_name: "Fixture · 198.51.100.20:443",
          network_code: 1,
          host: "fixture.example.test",
          upload_bytes_per_second: 12,
          download_bytes_per_second: 34,
        },
      ],
    };
  if (path === "/api/v1/runtime-sessions")
    return {
      sessions: [
        {
          started_at: 1_699_999_000,
          ended_at: null,
          start_reason: "startup",
          end_reason: null,
          last_seen_at: 1_700_000_000,
          sing_box_version: "sing-box 1.12.0",
          data_gap_before_seconds: 0,
        },
      ],
    };
  if (path === "/api/v1/overview")
    return {
      current: {
        upload_bytes: "10",
        download_bytes: "20",
        total_bytes: "30",
        elapsed_seconds: 60,
        observed_seconds: 60,
      },
      previous: {
        upload_bytes: "5",
        download_bytes: "10",
        total_bytes: "15",
        elapsed_seconds: 60,
        observed_seconds: 60,
      },
      boundary_approximate: false,
    };
  if (path === "/api/v1/series")
    return { boundary_approximate: false, points: [] };
  if (path === "/api/v1/quality") return { events: [] };
  if (path === "/api/v1/breakdown")
    return {
      by: "endpoint",
      available: true,
      approximate: true,
      boundary_approximate: false,
      no_traffic: false,
      connection_coverage: 1,
      dimension_retention: 1,
      global: { upload_bytes: "10", download_bytes: "20" },
      other: { upload_bytes: "0", download_bytes: "0" },
      unattributed: { upload_bytes: "0", download_bytes: "0" },
      items: [
        {
          raw_value: "198.51.100.20:443",
          display_name: "Fixture · 198.51.100.20:443",
          network_code: 1,
          upload_bytes: "10",
          download_bytes: "20",
        },
      ],
    };
  return undefined;
}
