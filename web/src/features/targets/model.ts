import type { BreakdownResponse, ByteString } from "../../api/contracts";
import { addByteStrings } from "../../lib/format";

export function buildTargetView(value: BreakdownResponse) {
  const items = value.items
    .map((item) => ({
      rawValue: item.raw_value,
      displayName: item.display_name,
      networkCode: item.network_code,
      uploadBytes: item.upload_bytes,
      downloadBytes: item.download_bytes,
      totalBytes: addByteStrings(item.upload_bytes, item.download_bytes),
    }))
    .sort(
      (left, right) =>
        compareBytes(right.totalBytes, left.totalBytes) ||
        left.rawValue.localeCompare(right.rawValue),
    );
  return {
    by: value.by,
    available: value.available,
    noTraffic: value.no_traffic,
    approximate: value.approximate,
    connectionCoverage: value.connection_coverage,
    dimensionRetention: value.dimension_retention,
    globalBytes: addByteStrings(
      value.global.upload_bytes,
      value.global.download_bytes,
    ),
    otherBytes: addByteStrings(
      value.other.upload_bytes,
      value.other.download_bytes,
    ),
    unattributedBytes: addByteStrings(
      value.unattributed.upload_bytes,
      value.unattributed.download_bytes,
    ),
    items,
  };
}

function compareBytes(left: ByteString, right: ByteString): number {
  const leftValue = BigInt(left);
  const rightValue = BigInt(right);
  return leftValue < rightValue ? -1 : leftValue > rightValue ? 1 : 0;
}
