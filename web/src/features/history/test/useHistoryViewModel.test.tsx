import { renderHook, waitFor } from "@testing-library/react";
import type { HistoricalRange, TimeSelection } from "../../../api/contracts";
import { DemoDataSource } from "../../../demo/source";
import { useHistoryViewModel } from "../useHistoryViewModel";

class FailingHistorySource extends DemoDataSource {
  fail = false;

  override async overview(range: HistoricalRange) {
    if (this.fail) throw new Error("fixture unavailable");
    return super.overview(range);
  }
}

class CountingHistorySource extends DemoDataSource {
  breakdownCalls = 0;

  override async breakdown(
    range: HistoricalRange,
    by: Parameters<DemoDataSource["breakdown"]>[1],
  ) {
    this.breakdownCalls += 1;
    return super.breakdown(range, by);
  }
}

describe("useHistoryViewModel", () => {
  it("clears the previous range when a new historical query fails", async () => {
    const source = new FailingHistorySource();
    const onUnauthorized = vi.fn();
    const initial: TimeSelection = { kind: "preset", preset: "today" };
    const { result, rerender } = renderHook(
      ({ selection }: { selection: TimeSelection }) =>
        useHistoryViewModel(
          source,
          selection,
          "Asia/Shanghai",
          "endpoint",
          onUnauthorized,
        ),
      { initialProps: { selection: initial } },
    );
    await waitFor(() => expect(result.current.view).not.toBeNull());

    source.fail = true;
    rerender({ selection: { kind: "preset", preset: "30d" } });

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.view).toBeNull();
    expect(result.current.breakdown).toBeNull();
  });

  it("reloads the active breakdown after an alias revision", async () => {
    const source = new CountingHistorySource();
    const onUnauthorized = vi.fn();
    const selection: TimeSelection = { kind: "preset", preset: "today" };
    const { rerender } = renderHook(
      ({ revision }: { revision: number }) =>
        useHistoryViewModel(
          source,
          selection,
          "Asia/Shanghai",
          "endpoint",
          onUnauthorized,
          revision,
        ),
      { initialProps: { revision: 0 } },
    );
    await waitFor(() => expect(source.breakdownCalls).toBe(1));

    rerender({ revision: 1 });

    await waitFor(() => expect(source.breakdownCalls).toBe(2));
  });
});
