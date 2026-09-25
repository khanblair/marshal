import { DAY_MS, MINUTE_MS } from "../constants";
import type { Profile } from "../settings-types";

/** The paired phone was last seen this many minutes before load. */
const PHONE_SEEN_MIN = 40;

export const seedProfile = (loadedAt: number): Profile => ({
  name: "Ada Okafor",
  email: "",
  tz: "Europe/London",
  avatar: null,
  tailnet: "ada@kolaborate.co",
  node: "marshal-laptop.tail3f2a.ts.net",
  devices: [
    { id: "d1", name: "Pixel 8", kind: "smartphone", last: loadedAt - PHONE_SEEN_MIN * MINUTE_MS },
    { id: "d2", name: "iPad Air", kind: "tablet", last: loadedAt - 2 * DAY_MS },
  ],
});
