import { act, renderHook, waitFor } from "@testing-library/react";
import type {
  HistoricalSelection,
  TimeSelection,
} from "../../../api/contracts";
import { DemoDataSource } from "../../../demo/source";
import { useHistoryViewModel } from "../useHistoryViewModel";

class FailingHistorySource extends DemoDataSource {
  fail = false;

  override async overview(range: HistoricalSelection) {
    if (this.fail) throw new Error("fixture unavailable");
    return super.overview(range);
  }
}

class CountingHistorySource extends DemoDataSource {
  breakdownCalls = 0;

  override async breakdown(
    range: HistoricalSelection,
    by: Parameters<DemoDataSource["breakdown"]>[1],
  ) {
    this.breakdownCalls += 1;
    return super.breakdown(range, by);
  }
}

describe("useHistoryViewModel", () => {
  it("retains the previous result while a new range loads or fails", async () => {
    const source = new FailingHistorySource();
    const onUnauthorized = vi.fn();
    const initial: TimeSelection = { kind: "preset", preset: "today" };
    const { result, rerender } = renderHook(
      ({ selection }: { selection: TimeSelection }) =>
        useHistoryViewModel(source, selection, "endpoint", onUnauthorized),
      { initialProps: { selection: initial } },
    );
    await waitFor(() => expect(result.current.view).not.toBeNull());

    const previous = result.current.view;
    source.fail = true;
    rerender({ selection: { kind: "preset", preset: "30d" } });

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.view).toBe(previous);
    expect(result.current.breakdown).not.toBeNull();
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

it("keeps a successful result visible until the next range is ready", async () => {
  const source = new DemoDataSource();
  const original = source.overview.bind(source);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.spyOn(source, "overview").mockImplementation(async (selection) => {
    if (selection.kind === "preset" && selection.preset === "30d") await gate;
    return original(selection);
  });
  const onUnauthorized = vi.fn();
  const { result, rerender } = renderHook(
    ({ selection }: { selection: TimeSelection }) =>
      useHistoryViewModel(source, selection, "endpoint", onUnauthorized),
    {
      initialProps: {
        selection: { kind: "preset", preset: "today" } as TimeSelection,
      },
    },
  );
  await waitFor(() => expect(result.current.view).not.toBeNull());
  const previous = result.current.view;
  rerender({ selection: { kind: "preset", preset: "30d" } });
  expect(result.current.loading).toBe(true);
  expect(result.current.view).toBe(previous);
  await act(async () => {
    release();
    await gate;
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.view).not.toBe(previous);
});
