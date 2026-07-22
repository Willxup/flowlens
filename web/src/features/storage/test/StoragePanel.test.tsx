import { render, screen } from "@testing-library/react";
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
  it("distinguishes capacity protection, failed cleanup and no cleanup", () => {
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

    expect(screen.getByText(/数据库已进入容量保护/)).toBeInTheDocument();
    expect(screen.getByText("失败")).toBeInTheDocument();
    expect(screen.queryByText(/空间充足/)).not.toBeInTheDocument();

    rerender(<StoragePanel value={base} />);
    expect(screen.getByText(/暂无聚合清理记录/)).toBeInTheDocument();
    expect(screen.getByText("暂无记录")).toBeInTheDocument();
  });
});
