import { useEffect, useMemo, useState } from "react";
import type {
  LabelCandidateResponse,
  LabelResponse,
} from "../../api/contracts";
import type { FlowLensDataSource } from "../../api/source";

export function AliasDialog({
  source,
  labels,
  candidates,
  onChanged,
  onClose,
}: {
  source: FlowLensDataSource;
  labels: LabelResponse[];
  candidates: LabelCandidateResponse[];
  onChanged: () => Promise<void>;
  onClose: () => void;
}) {
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [error, setError] = useState(false);
  const [saving, setSaving] = useState<string | null>(null);
  const entries = useMemo(() => {
    const byKey = new Map(
      candidates.map(
        (candidate) =>
          [
            key(candidate.label_type, candidate.match_value),
            candidate,
          ] as const,
      ),
    );
    const result: LabelCandidateResponse[] = [];
    const used = new Set<string>();
    for (const label of labels) {
      const itemKey = key(label.label_type, label.match_value);
      used.add(itemKey);
      result.push(
        byKey.get(itemKey) ?? {
          label_type: label.label_type,
          match_value: label.match_value,
          display_name: label.display_name,
          upload_bytes: "0" as LabelCandidateResponse["upload_bytes"],
          download_bytes: "0" as LabelCandidateResponse["download_bytes"],
        },
      );
    }
    for (const candidate of candidates)
      if (!used.has(key(candidate.label_type, candidate.match_value)))
        result.push(candidate);
    return result;
  }, [candidates, labels]);

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const candidate of entries) {
      const label = labels.find(
        (item) =>
          item.label_type === candidate.label_type &&
          item.match_value === candidate.match_value,
      );
      next[key(candidate.label_type, candidate.match_value)] =
        label?.display_name ?? "";
    }
    setEdits(next);
  }, [entries, labels]);

  async function save(
    candidate: LabelCandidateResponse,
    label: LabelResponse | undefined,
  ) {
    const itemKey = key(candidate.label_type, candidate.match_value);
    const displayName = (edits[itemKey] ?? "").trim();
    if (displayName === "" || source.demo) return;
    setSaving(itemKey);
    setError(false);
    try {
      if (label === undefined) {
        await source.createLabel({
          label_type: candidate.label_type,
          match_value: candidate.match_value,
          display_name: displayName,
        });
      } else {
        await source.updateLabel(label.id, displayName);
      }
      await onChanged();
    } catch {
      setError(true);
    } finally {
      setSaving(null);
    }
  }

  async function remove(label: LabelResponse) {
    if (source.demo) return;
    setSaving(key(label.label_type, label.match_value));
    setError(false);
    try {
      await source.deleteLabel(label.id);
      await onChanged();
    } catch {
      setError(true);
    } finally {
      setSaving(null);
    }
  }

  return (
    <div className="dialog-backdrop" role="presentation">
      <section
        className="dialog-shell"
        role="dialog"
        aria-modal="true"
        aria-labelledby="alias-title"
      >
        <header className="dialog-head">
          <div>
            <span className="eyebrow">Display aliases</span>
            <h2 id="alias-title">目标别名</h2>
            <p>别名只改变 FlowLens 展示，不修改代理服务或历史流量。</p>
          </div>
          <button type="button" aria-label="关闭别名" onClick={onClose}>
            ×
          </button>
        </header>
        {source.demo ? (
          <p className="demo-notice">Demo 为只读，别名修改仅在生产模式提供。</p>
        ) : null}
        {error ? (
          <p className="form-error" role="alert">
            别名保存失败，请重试。
          </p>
        ) : null}
        <div className="alias-list">
          {entries.map((candidate) => {
            const label = labels.find(
              (item) =>
                item.label_type === candidate.label_type &&
                item.match_value === candidate.match_value,
            );
            const itemKey = key(candidate.label_type, candidate.match_value);
            return (
              <div className="alias-row" key={itemKey}>
                <div>
                  <strong>{candidate.match_value}</strong>
                  <span>{candidate.label_type} 别名</span>
                </div>
                <input
                  aria-label={`${candidate.match_value} 显示名称`}
                  value={edits[itemKey] ?? ""}
                  onChange={(event) =>
                    setEdits((current) => ({
                      ...current,
                      [itemKey]: event.target.value,
                    }))
                  }
                  maxLength={64}
                  disabled={source.demo}
                />
                <div className="alias-actions">
                  <button
                    type="button"
                    aria-label={`${candidate.match_value} 保存`}
                    disabled={
                      source.demo ||
                      saving !== null ||
                      (edits[itemKey] ?? "").trim() === ""
                    }
                    onClick={() => void save(candidate, label)}
                  >
                    保存
                  </button>
                  {label === undefined ? null : (
                    <button
                      type="button"
                      aria-label={`${candidate.match_value} 删除`}
                      disabled={source.demo || saving !== null}
                      onClick={() => void remove(label)}
                    >
                      删除
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </section>
    </div>
  );
}

function key(type: string, value: string): string {
  return `${type}:${value}`;
}
