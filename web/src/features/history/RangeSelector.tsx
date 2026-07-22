import { useState } from "react";
import type { TimeSelection } from "../../api/contracts";

interface RangeSelectorProps {
  value: TimeSelection;
  timezone?: string;
  onChange: (value: TimeSelection) => void;
}

const presets: Array<{ label: string; value: TimeSelection }> = [
  { label: "实时", value: { kind: "live" } },
  { label: "今天", value: { kind: "preset", preset: "today" } },
  { label: "昨天", value: { kind: "preset", preset: "yesterday" } },
  { label: "7 天", value: { kind: "preset", preset: "7d" } },
  { label: "30 天", value: { kind: "preset", preset: "30d" } },
  { label: "90 天", value: { kind: "preset", preset: "90d" } },
  { label: "今年", value: { kind: "preset", preset: "year" } },
];
const weekdays = ["日", "一", "二", "三", "四", "五", "六"];

type DateField = "from" | "to";

interface CalendarMonth {
  year: number;
  month: number;
}

interface CalendarDay {
  value: string;
  day: number;
  currentMonth: boolean;
}

export function RangeSelector({
  value,
  timezone = "UTC",
  onChange,
}: RangeSelectorProps) {
  const [customOpen, setCustomOpen] = useState(false);
  const [from, setFrom] = useState("2026-07-01");
  const [to, setTo] = useState("2026-07-14");
  const [activeField, setActiveField] = useState<DateField>("from");
  const [calendarMonth, setCalendarMonth] = useState<CalendarMonth>({
    year: 2026,
    month: 7,
  });
  const selected = (candidate: TimeSelection) =>
    JSON.stringify(candidate) === JSON.stringify(value);
  const validCustomRange = from !== "" && to !== "" && from <= to;

  function openCustom() {
    if (value.kind === "custom") {
      setFrom(value.from);
      setTo(value.to);
      setCalendarMonth(monthFromDate(value.from));
    } else {
      setCalendarMonth(monthFromDate(from));
    }
    setActiveField("from");
    setCustomOpen(true);
  }

  function editField(field: DateField) {
    setActiveField(field);
    setCalendarMonth(monthFromDate(field === "from" ? from : to));
  }

  function selectDate(date: string) {
    if (activeField === "from") {
      setFrom(date);
      if (date > to) setTo(date);
      setActiveField("to");
      return;
    }
    if (date < from) setFrom(date);
    setTo(date);
  }

  const days = calendarDays(calendarMonth);

  return (
    <div className="range-wrap">
      <div className="segmented" aria-label="时间范围">
        {presets.map((preset) => (
          <button
            key={preset.label}
            type="button"
            aria-pressed={!customOpen && selected(preset.value)}
            onClick={() => {
              setCustomOpen(false);
              onChange(preset.value);
            }}
          >
            {preset.label}
          </button>
        ))}
        <button
          type="button"
          aria-pressed={customOpen || value.kind === "custom"}
          onClick={openCustom}
        >
          自定义
        </button>
      </div>
      {customOpen ? (
        <section
          className="custom-range-dialog"
          role="dialog"
          aria-labelledby="custom-range-title"
        >
          <header className="custom-range-head">
            <h2 id="custom-range-title">选择自定义日期</h2>
            <span>{timezone}</span>
          </header>
          <div className="custom-date-grid">
            <button
              className="custom-date-card"
              type="button"
              aria-label={`开始日期 ${from}`}
              aria-pressed={activeField === "from"}
              onClick={() => editField("from")}
            >
              <span>开始日期</span>
              <strong>{displayDate(from)}</strong>
            </button>
            <button
              className="custom-date-card"
              type="button"
              aria-label={`结束日期 ${to}`}
              aria-pressed={activeField === "to"}
              onClick={() => editField("to")}
            >
              <span>结束日期</span>
              <strong>{displayDate(to)}</strong>
            </button>
          </div>
          <section className="calendar-card" aria-label="日期选择器">
            <header className="calendar-head">
              <button
                type="button"
                aria-label="上个月"
                onClick={() => setCalendarMonth(shiftMonth(calendarMonth, -1))}
              >
                ‹
              </button>
              <h3>{`${calendarMonth.year} 年 ${calendarMonth.month} 月`}</h3>
              <button
                type="button"
                aria-label="下个月"
                onClick={() => setCalendarMonth(shiftMonth(calendarMonth, 1))}
              >
                ›
              </button>
            </header>
            <div className="calendar-weekdays" aria-hidden="true">
              {weekdays.map((day) => (
                <span key={day}>{day}</span>
              ))}
            </div>
            <div className="calendar-days">
              {days.map((date) => {
                const endpoint = date.value === from || date.value === to;
                const inRange = date.value >= from && date.value <= to;
                return (
                  <button
                    className={[
                      "calendar-day",
                      date.currentMonth ? "" : "outside-month",
                      inRange ? "in-range" : "",
                      endpoint ? "range-endpoint" : "",
                    ]
                      .filter(Boolean)
                      .join(" ")}
                    key={date.value}
                    type="button"
                    aria-label={date.value}
                    aria-pressed={endpoint}
                    onClick={() => {
                      selectDate(date.value);
                      if (!date.currentMonth)
                        setCalendarMonth(monthFromDate(date.value));
                    }}
                  >
                    {date.day}
                  </button>
                );
              })}
            </div>
          </section>
          <footer className="custom-range-actions">
            <span>{`${displayDate(from)} — ${displayDate(to)}`}</span>
            <div>
              <button
                className="soft-button"
                type="button"
                onClick={() => setCustomOpen(false)}
              >
                取消
              </button>
              <button
                className="primary-button"
                type="button"
                disabled={!validCustomRange}
                onClick={() => {
                  onChange({ kind: "custom", from, to });
                  setCustomOpen(false);
                }}
              >
                应用
              </button>
            </div>
          </footer>
        </section>
      ) : null}
    </div>
  );
}

function monthFromDate(value: string): CalendarMonth {
  const { year, month } = dateParts(value);
  return { year, month };
}

function shiftMonth(value: CalendarMonth, offset: number): CalendarMonth {
  const date = new Date(Date.UTC(value.year, value.month - 1 + offset, 1));
  return { year: date.getUTCFullYear(), month: date.getUTCMonth() + 1 };
}

function calendarDays(value: CalendarMonth): CalendarDay[] {
  const first = new Date(Date.UTC(value.year, value.month - 1, 1));
  const startDay = 1 - first.getUTCDay();
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(
      Date.UTC(value.year, value.month - 1, startDay + index),
    );
    return {
      value: isoDate(date),
      day: date.getUTCDate(),
      currentMonth: date.getUTCMonth() === value.month - 1,
    };
  });
}

function isoDate(value: Date): string {
  return [
    value.getUTCFullYear(),
    String(value.getUTCMonth() + 1).padStart(2, "0"),
    String(value.getUTCDate()).padStart(2, "0"),
  ].join("-");
}

function displayDate(value: string): string {
  const { year, month, day } = dateParts(value);
  return `${year}年${month}月${day}日`;
}

function dateParts(value: string): {
  year: number;
  month: number;
  day: number;
} {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) throw new Error("invalid date");
  return {
    year: Number(value.slice(0, 4)),
    month: Number(value.slice(5, 7)),
    day: Number(value.slice(8, 10)),
  };
}
