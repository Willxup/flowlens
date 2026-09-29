import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { DemoDataSource } from "../../../demo/source";
import { LiveTrafficExplorer } from "../LiveTrafficExplorer";
vi.mock("../TrafficChart", () => ({
  TrafficChart: ({ liveBounds }: { liveBounds: readonly number[] }) => (
    <output aria-label="chart bounds">{liveBounds.join(",")}</output>
  ),
}));
it("keeps historical selection fixed on updates and resumes a rolling hour on request", async () => {
  const source = new DemoDataSource();
  let now = source.now().getTime() / 1000;
  vi.spyOn(source, "now").mockImplementation(() => new Date(now * 1000));
  const query = vi.spyOn(source, "series");
  const props = { source, live: [], onUnauthorized: vi.fn() };
  const view = render(<LiveTrafficExplorer {...props} />);
  await waitFor(() =>
    expect(query).toHaveBeenCalledWith(
      { kind: "preset", preset: "24h" },
      expect.any(AbortSignal),
    ),
  );
  expect(screen.getByLabelText("chart bounds")).toHaveTextContent(
    `${now - 3600},${now}`,
  );
  fireEvent.change(screen.getByRole("slider", { name: "预览结束时间" }), {
    target: { value: now - 7200 },
  });
  const paused = screen.getByLabelText("chart bounds").textContent;
  now += 120;
  view.rerender(<LiveTrafficExplorer {...props} />);
  expect(screen.getByLabelText("chart bounds").textContent).toBe(paused);
  expect(screen.getByText("浏览历史 · 已暂停跟随")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "回到实时" }));
  expect(screen.getByLabelText("chart bounds")).toHaveTextContent(
    `${now - 3600},${now}`,
  );
  fireEvent.click(screen.getByRole("button", { name: "查看 24 小时" }));
  expect(screen.getByLabelText("chart bounds")).toHaveTextContent(
    `${now - 86400},${now}`,
  );
});
it("reports archive failures independently and retries without losing the live view", async () => {
  const source = new DemoDataSource();
  const query = vi
    .spyOn(source, "series")
    .mockRejectedValueOnce(new Error("offline"));
  render(
    <LiveTrafficExplorer source={source} live={[]} onUnauthorized={vi.fn()} />,
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "24 小时历史加载失败",
  );
  expect(screen.getByLabelText("chart bounds")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  await waitFor(() =>
    expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
  );
  expect(query).toHaveBeenCalledTimes(2);
});
