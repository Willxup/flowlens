import type { HistoricalRange, TimeSelection } from "../api/contracts";

interface DateParts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

const daySeconds = 86_400;
const formatters = new Map<string, Intl.DateTimeFormat>();

export function toHistoricalRange(
  selection: TimeSelection,
  now: Date,
  timezone: string,
): HistoricalRange {
  if (selection.kind === "live")
    throw new Error("live selection is not historical");
  const to = Math.floor(now.getTime() / 1000);
  if (!Number.isSafeInteger(to) || to <= 1)
    throw new Error("invalid current time");
  if (selection.kind === "custom") {
    const fromDate = parseDate(selection.from);
    const endDate = parseDate(selection.to);
    if (
      fromDate === null ||
      endDate === null ||
      compareDate(fromDate, endDate) > 0
    ) {
      throw new Error("invalid custom range");
    }
    const from = zonedMidnight(fromDate, timezone);
    const customTo = zonedMidnight(addDays(endDate, 1), timezone);
    if (customTo <= from) throw new Error("invalid custom range");
    return { from, to: customTo };
  }
  switch (selection.preset) {
    case "7d":
      return { from: to - 7 * daySeconds, to };
    case "30d":
      return { from: to - 30 * daySeconds, to };
    case "90d":
      return { from: to - 90 * daySeconds, to };
    case "lifetime":
      return { from: daySeconds, to };
  }
  const local = dateParts(now, timezone);
  const currentDate = { year: local.year, month: local.month, day: local.day };
  if (selection.preset === "today") {
    return { from: zonedMidnight(currentDate, timezone), to };
  }
  if (selection.preset === "yesterday") {
    const yesterday = addDays(currentDate, -1);
    return {
      from: zonedMidnight(yesterday, timezone),
      to: zonedMidnight(currentDate, timezone),
    };
  }
  return {
    from: zonedMidnight({ year: local.year, month: 1, day: 1 }, timezone),
    to,
  };
}

function formatter(timezone: string): Intl.DateTimeFormat {
  let value = formatters.get(timezone);
  if (value !== undefined) return value;
  value = new Intl.DateTimeFormat("en-CA", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });
  formatters.set(timezone, value);
  return value;
}

function dateParts(date: Date, timezone: string): DateParts {
  const parts = formatter(timezone).formatToParts(date);
  const number = (type: Intl.DateTimeFormatPartTypes) => {
    const part = parts.find((entry) => entry.type === type);
    if (part === undefined) throw new Error("invalid timezone result");
    return Number(part.value);
  };
  return {
    year: number("year"),
    month: number("month"),
    day: number("day"),
    hour: number("hour"),
    minute: number("minute"),
    second: number("second"),
  };
}

function zonedMidnight(
  value: Pick<DateParts, "year" | "month" | "day">,
  timezone: string,
): number {
  const target = Date.UTC(value.year, value.month - 1, value.day);
  let candidate = target;
  for (let attempt = 0; attempt < 4; attempt++) {
    const rendered = dateParts(new Date(candidate), timezone);
    const renderedUTC = Date.UTC(
      rendered.year,
      rendered.month - 1,
      rendered.day,
      rendered.hour,
      rendered.minute,
      rendered.second,
    );
    const next = target - (renderedUTC - candidate);
    if (next === candidate) break;
    candidate = next;
  }
  const check = dateParts(new Date(candidate), timezone);
  if (
    check.year !== value.year ||
    check.month !== value.month ||
    check.day !== value.day ||
    check.hour !== 0 ||
    check.minute !== 0
  ) {
    throw new Error("invalid timezone boundary");
  }
  return Math.floor(candidate / 1000);
}

function parseDate(
  value: string,
): Pick<DateParts, "year" | "month" | "day"> | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (match === null) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const check = new Date(Date.UTC(year, month - 1, day));
  if (
    check.getUTCFullYear() !== year ||
    check.getUTCMonth() !== month - 1 ||
    check.getUTCDate() !== day
  )
    return null;
  return { year, month, day };
}

function addDays(
  value: Pick<DateParts, "year" | "month" | "day">,
  days: number,
) {
  const date = new Date(
    Date.UTC(value.year, value.month - 1, value.day + days),
  );
  return {
    year: date.getUTCFullYear(),
    month: date.getUTCMonth() + 1,
    day: date.getUTCDate(),
  };
}

function compareDate(
  left: Pick<DateParts, "year" | "month" | "day">,
  right: Pick<DateParts, "year" | "month" | "day">,
): number {
  return (
    Date.UTC(left.year, left.month - 1, left.day) -
    Date.UTC(right.year, right.month - 1, right.day)
  );
}
