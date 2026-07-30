import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeSelect } from "../ThemeSelect";

describe("ThemeSelect", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("uses accessible SVG buttons and persists explicit themes", async () => {
    const user = userEvent.setup();
    render(<ThemeSelect />);
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    const system = screen.getByRole("button", { name: "跟随系统" });
    const light = screen.getByRole("button", { name: "浅色模式" });
    const dark = screen.getByRole("button", { name: "深色模式" });
    expect(system).toHaveAttribute("aria-pressed", "true");
    expect(system).not.toHaveAttribute("title");
    expect(system.querySelector("svg")).toBeInTheDocument();
    expect(light.querySelector("svg")).toBeInTheDocument();
    expect(dark.querySelector("svg")).toBeInTheDocument();

    await user.hover(system);
    expect(screen.getByRole("tooltip")).toHaveTextContent("跟随系统");
    await user.click(dark);
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(localStorage.getItem("flowlens-theme")).toBe("dark");
    await user.click(light);
    expect(document.documentElement.dataset.theme).toBe("light");
    await user.click(system);
    expect(document.documentElement).not.toHaveAttribute("data-theme");
    expect(localStorage.getItem("flowlens-theme")).toBe("system");
  });

  it("discards invalid stored values", () => {
    localStorage.setItem("flowlens-theme", "neon");
    render(<ThemeSelect />);
    expect(screen.getByRole("button", { name: "跟随系统" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(localStorage.getItem("flowlens-theme")).toBeNull();
  });
});
