import type { BreakdownResponse } from "../../../api/contracts";
import { asByteString } from "../../../lib/format";
import { buildTargetView } from "../model";

describe("buildTargetView", () => {
  it("sorts with BigInt and keeps approximation/coverage visible", () => {
    const breakdown: BreakdownResponse = {
      by: "endpoint",
      available: true,
      approximate: true,
      boundary_approximate: false,
      no_traffic: false,
      connection_coverage: 0.9,
      dimension_retention: 0.8,
      global: {
        upload_bytes: asByteString("9007199254741000"),
        download_bytes: asByteString("0"),
      },
      other: {
        upload_bytes: asByteString("10"),
        download_bytes: asByteString("0"),
      },
      unattributed: {
        upload_bytes: asByteString("20"),
        download_bytes: asByteString("0"),
      },
      items: [
        {
          raw_value: "198.51.100.1:443",
          display_name: "Small",
          network_code: 1,
          upload_bytes: asByteString("9"),
          download_bytes: asByteString("0"),
        },
        {
          raw_value: "203.0.113.1:443",
          display_name: "Large · 203.0.113.1:443",
          network_code: 1,
          upload_bytes: asByteString("9007199254740993"),
          download_bytes: asByteString("0"),
        },
      ],
    };
    const view = buildTargetView(breakdown);
    expect(view.items[0]?.rawValue).toBe("203.0.113.1:443");
    expect(view.approximate).toBe(true);
    expect(view.connectionCoverage).toBe(0.9);
    expect(view.otherBytes).toBe("10");
    expect(view.unattributedBytes).toBe("20");
  });
});
