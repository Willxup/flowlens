import type { ByteString } from "../api/contracts";

const bytePattern = /^(0|[1-9][0-9]*)$/;
const byteUnits = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"] as const;

export function asByteString(value: string): ByteString {
  if (!bytePattern.test(value)) throw new Error("invalid byte string");
  return value as ByteString;
}

export function addByteStrings(...values: ByteString[]): ByteString {
  return asByteString(
    values.reduce((sum, value) => sum + BigInt(value), 0n).toString(),
  );
}

export function formatBytes(value: ByteString): string {
  const bytes = BigInt(value);
  let scale = 1n;
  let unit = 0;
  while (unit < byteUnits.length - 1 && bytes >= scale * 1024n) {
    scale *= 1024n;
    unit++;
  }
  if (unit === 0) return `${bytes.toString()} B`;
  const tenths = (bytes * 10n + scale / 2n) / scale;
  const whole = tenths / 10n;
  const decimal = tenths % 10n;
  return `${whole.toString()}${decimal === 0n ? "" : `.${decimal.toString()}`} ${byteUnits[unit]}`;
}

export function formatRate(value: number | null): string {
  if (value === null || !Number.isFinite(value) || value < 0) return "—";
  const units = ["B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s"];
  let scaled = value;
  let unit = 0;
  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024;
    unit++;
  }
  const digits = scaled >= 10 || Number.isInteger(scaled) ? 0 : 1;
  return `${scaled.toFixed(digits)} ${units[unit]}`;
}

export function formatRatio(value: number | null): string {
  if (value === null || !Number.isFinite(value) || value < 0 || value > 1)
    return "—";
  return `${(value * 100).toFixed(1).replace(/\.0$/, "")}%`;
}

export function formatNetwork(code: number): string {
  if (code === 1) return "TCP";
  if (code === 2) return "UDP";
  return "未知协议";
}
