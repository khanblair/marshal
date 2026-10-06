import type { AuthorizeURL, GoogleCalendarChoices, GoogleClientInfo } from "@marshal/protocol";
import { ApiError } from "~/data/api-error";
import type { Ctx } from "~/mock/context";
import { applyIntegrationList } from "./integrations";

/*
 * Google's two connections beyond saving the client (sections S29d and S29e): Gmail's own consent,
 * and the choice of which calendars Marshal reads. Their words are the daemon's own: a refusal such
 * as "Google Calendar is not connected yet" is already a sentence a person can act on.
 */

export const NO_DAEMON = "Marshal is not connected to its daemon.";
const FAILED = "Marshal could not reach Google. Try again.";

/** The calendars Google lists, or the sentence that says why they could not be read. */
export type GoogleCalendarsAnswer = { choices: GoogleCalendarChoices } | { error: string };

/** The daemon's own sentence for a refusal, or the plain one when Google could not be reached. */
export function sentence(error: unknown): string {
  return error instanceof ApiError ? error.message : FAILED;
}

/** Gmail's consent URL, for the owner's own browser to open. Null when the daemon refused it. */
export async function authorizeGmail(c: Ctx): Promise<AuthorizeURL | null> {
  const api = c.env.data?.api;
  if (!api) return null;
  try {
    return await api.authorizeGmail();
  } catch {
    return null;
  }
}

/** Every calendar the owner has, with whether Marshal reads it. */
export async function googleCalendars(c: Ctx): Promise<GoogleCalendarsAnswer> {
  const api = c.env.data?.api;
  if (!api) return { error: NO_DAEMON };
  try {
    return { choices: await api.googleCalendars() };
  } catch (error) {
    return { error: sentence(error) };
  }
}

/** Chooses the calendars Marshal reads, and answers the list as it now stands. */
export async function setGoogleCalendars(c: Ctx, ids: string[]): Promise<GoogleCalendarsAnswer> {
  const api = c.env.data?.api;
  if (!api) return { error: NO_DAEMON };
  try {
    return { choices: await api.setGoogleCalendars({ ids }) };
  } catch (error) {
    return { error: sentence(error) };
  }
}

/**
 * Whether this build has Marshal's own Google client, so Connect with Google is one click, and
 * whether the person saved a client of their own. Null when the daemon could not say, and then the
 * screen offers the paste form, which always works.
 */
export async function googleClientInfo(c: Ctx): Promise<GoogleClientInfo | null> {
  const api = c.env.data?.api;
  if (!api) return null;
  try {
    return await api.googleClient();
  } catch {
    return null;
  }
}

/**
 * Reads the connection list again. A person who grants access in Google's own tab comes back to a
 * row that has not heard of it, because nothing tells this page, so the screen asks while it waits.
 * A read that fails leaves the rows as they were.
 */
export async function refreshIntegrationList(c: Ctx): Promise<void> {
  const api = c.env.data?.api;
  if (!api) return;
  try {
    applyIntegrationList(c, await api.listIntegrations());
  } catch {
    // The list is read again with the next change.
  }
}
