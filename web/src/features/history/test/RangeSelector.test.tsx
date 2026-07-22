import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RangeSelector } from "../RangeSelector";

describe("RangeSelector", () => {
  it("temporarily hides the all-data range", () => {
    const onChange = vi.fn();
    render(<RangeSelector value={{ kind: "live" }} onChange={onChange} />);
    expect(screen.queryByText("生命周期")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "全部" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "今年" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "自定义" })).toBeInTheDocument();
  });

  it("marks custom as selected while its dialog is open", async () => {
    const user = userEvent.setup();
    render(
      <RangeSelector
        value={{ kind: "preset", preset: "90d" }}
        onChange={vi.fn()}
      />,
    );

    const custom = screen.getByRole("button", { name: "自定义" });
    await user.click(custom);

    expect(custom).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "90 天" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(
      screen.getByRole("dialog", { name: "选择自定义日期" }),
    ).toBeInTheDocument();
  });

  it("restores the applied preset when custom editing is cancelled", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <RangeSelector
        value={{ kind: "preset", preset: "30d" }}
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "自定义" }));
    await user.click(screen.getByRole("button", { name: "取消" }));

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "30 天" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("selects and applies dates through date cards and the calendar", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<RangeSelector value={{ kind: "live" }} onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "自定义" }));
    const start = screen.getByRole("button", {
      name: "开始日期 2026-07-01",
    });
    const end = screen.getByRole("button", {
      name: "结束日期 2026-07-14",
    });
    expect(start).toHaveAttribute("aria-pressed", "true");
    expect(end).toHaveAttribute("aria-pressed", "false");
    expect(screen.queryByLabelText("开始日期")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "2026-07-16" }));
    expect(end).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: "2026-07-22" }));
    await user.click(screen.getByRole("button", { name: "应用" }));

    expect(onChange).toHaveBeenCalledWith({
      kind: "custom",
      from: "2026-07-16",
      to: "2026-07-22",
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("moves the calendar between months", async () => {
    const user = userEvent.setup();
    render(<RangeSelector value={{ kind: "live" }} onChange={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "自定义" }));
    expect(
      screen.getByRole("heading", { name: "2026 年 7 月" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "下个月" }));
    expect(
      screen.getByRole("heading", { name: "2026 年 8 月" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "上个月" }));
    expect(
      screen.getByRole("heading", { name: "2026 年 7 月" }),
    ).toBeInTheDocument();
  });
});
