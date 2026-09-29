import { describe, expect, it } from "vitest";
import raw from "../../../daemon/testdata/golden/schedule.json";
import type { Schedule } from "../src/generated";

describe("schedule golden", () => {
  it("matches the daemon's golden file", () => {
    const s: Schedule = raw;
    expect(s.id).toBe("01H1234567890ABCDEFGHJKMNP");
    expect(s.name).toBe("Morning brief");
    expect(s.kind).toBe("brief");
    expect(s.enabled).toBe(true);
  });
});
