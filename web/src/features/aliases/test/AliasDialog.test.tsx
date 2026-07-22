import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type {
  LabelCandidateResponse,
  LabelResponse,
} from "../../../api/contracts";
import { DemoDataSource } from "../../../demo/source";
import { asByteString } from "../../../lib/format";
import { AliasDialog } from "../AliasDialog";

const candidate: LabelCandidateResponse = {
  label_type: "endpoint",
  match_value: "198.51.100.20:443",
  display_name: "198.51.100.20:443",
  upload_bytes: asByteString("10"),
  download_bytes: asByteString("20"),
};

describe("AliasDialog", () => {
  it("creates, updates and deletes aliases through the server source of truth", async () => {
    const source = new DemoDataSource();
    Object.defineProperty(source, "demo", { value: false });
    const create = vi.spyOn(source, "createLabel").mockResolvedValue({
      id: 1,
      label_type: "endpoint",
      match_value: candidate.match_value,
      display_name: "Media",
      created_at: 1,
      updated_at: 1,
    });
    const changed = vi.fn(async () => undefined);
    const { rerender } = render(
      <AliasDialog
        source={source}
        labels={[]}
        candidates={[candidate]}
        onChanged={changed}
        onClose={vi.fn()}
      />,
    );
    const input = screen.getByLabelText(`${candidate.match_value} 显示名称`);
    await userEvent.type(input, "Media");
    await userEvent.click(
      screen.getByRole("button", { name: `${candidate.match_value} 保存` }),
    );
    expect(create).toHaveBeenCalledWith({
      label_type: "endpoint",
      match_value: candidate.match_value,
      display_name: "Media",
    });
    expect(changed).toHaveBeenCalledOnce();

    const label: LabelResponse = {
      id: 1,
      label_type: "endpoint",
      match_value: candidate.match_value,
      display_name: "Media",
      created_at: 1,
      updated_at: 1,
    };
    const update = vi
      .spyOn(source, "updateLabel")
      .mockResolvedValue({ ...label, display_name: "Streaming" });
    const remove = vi.spyOn(source, "deleteLabel").mockResolvedValue();
    rerender(
      <AliasDialog
        source={source}
        labels={[label]}
        candidates={[candidate]}
        onChanged={changed}
        onClose={vi.fn()}
      />,
    );
    const existing = screen.getByLabelText(`${candidate.match_value} 显示名称`);
    await userEvent.clear(existing);
    await userEvent.type(existing, "Streaming");
    await userEvent.click(
      screen.getByRole("button", { name: `${candidate.match_value} 保存` }),
    );
    expect(update).toHaveBeenCalledWith(1, "Streaming");
    await userEvent.click(
      screen.getByRole("button", { name: `${candidate.match_value} 删除` }),
    );
    expect(remove).toHaveBeenCalledWith(1);
  });

  it("does not replace server values after a failed write", async () => {
    const source = new DemoDataSource();
    Object.defineProperty(source, "demo", { value: false });
    vi.spyOn(source, "createLabel").mockRejectedValue(
      new Error("private detail"),
    );
    render(
      <AliasDialog
        source={source}
        labels={[]}
        candidates={[candidate]}
        onChanged={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    await userEvent.type(
      screen.getByLabelText(`${candidate.match_value} 显示名称`),
      "Media",
    );
    await userEvent.click(
      screen.getByRole("button", { name: `${candidate.match_value} 保存` }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "别名保存失败，请重试。",
    );
    expect(
      screen.getByLabelText(`${candidate.match_value} 显示名称`),
    ).toHaveValue("Media");
  });

  it("keeps existing aliases manageable after they leave the candidate window", async () => {
    const source = new DemoDataSource();
    Object.defineProperty(source, "demo", { value: false });
    const label: LabelResponse = {
      id: 9,
      label_type: "endpoint",
      match_value: "203.0.113.44:443",
      display_name: "Archive",
      created_at: 1,
      updated_at: 2,
    };
    const remove = vi.spyOn(source, "deleteLabel").mockResolvedValue();

    render(
      <AliasDialog
        source={source}
        labels={[label]}
        candidates={[]}
        onChanged={vi.fn(async () => undefined)}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByLabelText(`${label.match_value} 显示名称`)).toHaveValue(
      "Archive",
    );
    await userEvent.click(
      screen.getByRole("button", { name: `${label.match_value} 删除` }),
    );
    expect(remove).toHaveBeenCalledWith(label.id);
  });
});
