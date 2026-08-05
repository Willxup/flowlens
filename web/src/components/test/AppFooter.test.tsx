import { render, screen } from "@testing-library/react";
import { AppFooter, footerVersionLabel } from "../AppFooter";

describe("AppFooter", () => {
  it("renders the project, license, author, and injected version links", () => {
    render(<AppFooter version="v0.2.5" />);

    expect(screen.getByText("© 2026")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "FlowLens" })).toHaveAttribute(
      "href",
      "https://github.com/Willxup/flowlens",
    );
    expect(screen.getByRole("link", { name: "License" })).toHaveAttribute(
      "href",
      "https://github.com/Willxup/flowlens/blob/main/LICENSE",
    );
    expect(
      screen.getByRole("link", { name: "Willxup GitHub 主页" }),
    ).toHaveAttribute("href", "https://github.com/Willxup");
    expect(screen.getByText("Powered By")).toBeInTheDocument();
    expect(screen.getByText("Version: v0.2.5")).toBeInTheDocument();
  });

  it("omits the version label until a non-empty build version is available", () => {
    const { rerender } = render(<AppFooter />);
    expect(screen.queryByText(/Version:/)).not.toBeInTheDocument();

    rerender(<AppFooter version="   " />);
    expect(screen.queryByText(/Version:/)).not.toBeInTheDocument();
    expect(footerVersionLabel(" v0.2.5 ")).toBe("Version: v0.2.5");
  });
});
