import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FlowLensDataSource } from "../../../api/source";
import { DemoDataSource } from "../../../demo/source";
import { LoginPage } from "../LoginPage";

describe("LoginPage", () => {
  it("contains only the shared-key login flow", async () => {
    const source = new DemoDataSource();
    const login = vi.spyOn(source, "login");
    const authenticated = vi.fn();
    render(<LoginPage source={source} onAuthenticated={authenticated} />);

    expect(screen.queryByLabelText(/用户名/)).not.toBeInTheDocument();
    expect(screen.queryByText(/注册|找回密码/)).not.toBeInTheDocument();
    const button = screen.getByRole("button", { name: "进入 FlowLens" });
    expect(button).toBeDisabled();

    const key = screen.getByLabelText("共享访问密钥");
    await userEvent.type(key, "fixture-access-key");
    await userEvent.click(button);

    expect(login).toHaveBeenCalledWith("fixture-access-key");
    expect(authenticated).toHaveBeenCalledOnce();
    expect(key).toHaveValue("");
  });

  it("uses one generic failure message", async () => {
    const source = new DemoDataSource() as FlowLensDataSource;
    vi.spyOn(source, "login").mockRejectedValue(
      new Error("private backend detail"),
    );
    render(<LoginPage source={source} onAuthenticated={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("共享访问密钥"), "wrong-key");
    await userEvent.click(
      screen.getByRole("button", { name: "进入 FlowLens" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "无法登录，请检查密钥后重试。",
    );
    expect(screen.getByRole("alert")).not.toHaveTextContent(
      "private backend detail",
    );
  });
});
