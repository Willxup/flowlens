import {
  addByteStrings,
  asByteString,
  formatBytes,
  formatRate,
  formatRatio,
} from "../format";

describe("byte formatting", () => {
  it("keeps exact decimal strings above Number safe range", () => {
    const upload = asByteString("9007199254740993");
    const download = asByteString("7");
    expect(addByteStrings(upload, download)).toBe("9007199254741000");
    expect(formatBytes(upload)).toBe("8 PiB");
  });

  it("rejects non-canonical or negative byte strings", () => {
    for (const value of ["", "-1", "+1", "1.5", "01", " 1", "NaN"]) {
      expect(() => asByteString(value)).toThrow("invalid byte string");
    }
  });

  it("formats rates and nullable ratios without claiming missing values are zero", () => {
    expect(formatRate(1536)).toBe("1.5 KiB/s");
    expect(formatRate(null)).toBe("—");
    expect(formatRatio(0.924)).toBe("92.4%");
    expect(formatRatio(null)).toBe("—");
  });
});
