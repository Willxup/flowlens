import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InfoTooltip, Tooltip } from "../Tooltip";

describe("Tooltip", () => {
  it("shows custom content immediately on hover and removes it on leave", async () => {
    const user = userEvent.setup();
    render(
      <Tooltip content="即时说明">
        <button type="button">目标</button>
      </Tooltip>,
    );

    const trigger = screen.getByRole("button", { name: "目标" });
    expect(trigger).not.toHaveAttribute("title");
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    await user.hover(trigger);
    expect(screen.getByRole("tooltip")).toHaveTextContent("即时说明");
    expect(trigger).toHaveAttribute("aria-describedby");

    await user.unhover(trigger);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });

  it("supports keyboard focus and Escape", async () => {
    const user = userEvent.setup();
    render(<InfoTooltip content="键盘说明" label="查看参数说明" />);

    await user.tab();
    expect(screen.getByRole("tooltip")).toHaveTextContent("键盘说明");

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });
});
