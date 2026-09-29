/* Settings entities: roles, provider keys, integrations, schedules, calendar, profile. */

import type { IntegrationTest } from "~/data/mappers/integrations";

/**
 * A provider row: the daemon's, mapped by `~/data/mappers/providers.ts`. It is defined there, not
 * here, so the mapper never depends on the mock, and re-exported here because the screens import
 * their store types from `~/mock`.
 */
export type {
  ProviderRow as Provider,
  ProviderTest,
  ProviderTestCheck,
} from "~/data/mappers/providers";
/**
 * A role row: the daemon's, mapped by `~/data/mappers/roles.ts`. It is defined there, not here, so
 * the mapper never depends on the mock, and re-exported here because the screens import their store
 * types from `~/mock`.
 */
export type { Role } from "~/data/mappers/roles";
export interface Integration {
  id: string;
  name: string;
  icon: string;
  st: "connected" | "none" | "error";
  detail: string;
  /**
   * The last connection test's result, or absent when the connection was never tested. It is the
   * daemon's, filled by `sync/integrations.ts` for a connection whose section is switched, and the
   * empty form is dropped so the screen can ask `if (row.lastTest)`.
   */
  lastTest?: IntegrationTest;
}
/**
 * A calendar event: the daemon's, mapped by `~/data/mappers/calendar.ts`. It is defined there, not
 * here, so the mapper never depends on the mock, and re-exported here because the screens import
 * their store types from `~/mock`.
 */
export type { CalEvent } from "~/data/mappers/calendar";
/**
 * A schedule row: the daemon's, mapped by `~/data/mappers/schedules.ts`. It is defined there, not
 * here, so the mapper never depends on the mock, and re-exported here because the screens import
 * their store types from `~/mock`.
 */
export type { ScheduleRow as Schedule } from "~/data/mappers/schedules";

interface Device {
  id: string;
  name: string;
  kind: string;
  last: number;
  /**
   * True for a device whose token was revoked. The row stays in the list so the screen can say one
   * was removed rather than watching it vanish (B9.1); the mock's own rows are never revoked.
   */
  revoked?: boolean;
}
export interface Profile {
  /** The daemon's id for the person. The mock's profile has none. */
  id?: string;
  /** The initials the daemon made from the name. The mock's profile has none: the screens work them out. */
  initials?: string;
  name: string;
  email: string;
  tz: string;
  avatar: string | null;
  tailnet: string;
  node: string;
  devices: Device[];
}
