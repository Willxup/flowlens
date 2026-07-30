import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { StorageResponse } from "../../../api/contracts";
import { asByteString } from "../../../lib/format";
import { StoragePanel } from "../StoragePanel";

const base: StorageResponse = {
  database_bytes: asByteString("1024"),
  wal_bytes: asByteString("0"),
  soft_limit_bytes: asByteString("2048"),
  protecting: false,
  last_rollup_cleanup: null,
};

describe("StoragePanel", () => {
  it("moves the current storage summary into a tooltip", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <StoragePanel
        value={{
          ...base,
          protecting: true,
          last_rollup_cleanup: {
            started_at: 10,
            ended_at: 20,
            deleted_rows: 0,
            successful: false,
          },
        }}
      />,
    );

    expect(screen.queryByText(/数据库已进入容量保护/)).not.toBeInTheDocument();
    expect(screen.getByText("失败")).toBeInTheDocument();
    expect(screen.queryByText(/空间充足/)).not.toBeInTheDocument();
    const trigger = screen.getByRole("button", {
      name: "查看“存储健康”说明",
    });
    await user.hover(trigger);
    expect(screen.getByRole("tooltip")).toHaveTextContent(
      "数据库已进入容量保护，请检查空间和保留策略。",
    );
    await user.unhover(trigger);

    rerender(<StoragePanel value={base} />);
    expect(screen.queryByText(/暂无聚合清理记录/)).not.toBeInTheDocument();
    expect(screen.getByText("暂无记录")).toBeInTheDocument();
    await user.hover(trigger);
    expect(screen.getByRole("tooltip")).toHaveTextContent(
      "数据库空间正常，暂无聚合清理记录。",
    );
  });
});
