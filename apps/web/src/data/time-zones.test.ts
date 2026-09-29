import { describe, expect, it } from "vitest";
import { allTimeZones, currentTimeZone, zoneChoices } from "./time-zones";

describe("time zones", () => {
  it("lists every zone the browser knows, with UTC first", () => {
    const zones = allTimeZones();
    expect(zones.length).toBeGreaterThan(100);
    expect(zones[0]).toBe("UTC");
    expect(zones).toContain("Africa/Nairobi");
  });

  it("reads the zone of this device", () => {
    expect(currentTimeZone()).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone);
  });

  it("keeps a chosen zone the list does not have", () => {
    expect(zoneChoices("Mars/Olympus")[0]).toBe("Mars/Olympus");
    expect(zoneChoices("Europe/London")).not.toContain("Mars/Olympus");
  });
});
