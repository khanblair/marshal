/** Zones to offer when the browser cannot list the time zone database. */
const FALLBACK = [
  "Europe/London",
  "Europe/Lisbon",
  "Europe/Paris",
  "Africa/Lagos",
  "Africa/Nairobi",
  "America/New_York",
  "America/Chicago",
  "America/Los_Angeles",
  "Asia/Dubai",
  "Asia/Kolkata",
  "Asia/Singapore",
  "Asia/Tokyo",
  "Australia/Sydney",
] as const;

/** Every zone of the time zone database the browser knows, with UTC first. */
export function allTimeZones(): string[] {
  let zones: string[] = [];
  try {
    zones = Intl.supportedValuesOf("timeZone");
  } catch {
    zones = [];
  }
  const list = zones.length > 0 ? zones : [...FALLBACK];
  return list.includes("UTC") ? list : ["UTC", ...list];
}

/** The time zone of the device the person is on now, or empty when it cannot be read. */
export function currentTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {
    return "";
  }
}

/** The zones a picker offers: all of them, and the one already chosen if it is somehow not among them. */
export function zoneChoices(current: string): string[] {
  const zones = allTimeZones();
  return current && !zones.includes(current) ? [current, ...zones] : zones;
}
