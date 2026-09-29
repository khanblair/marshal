import type {
  AuthorizeURL,
  DetectTelegramChatAnswer,
  DetectTelegramChatRequest,
  IntegrationList,
  SaveDiscordRequest,
  SaveGitHubRequest,
  SaveGmailRequest,
  SaveGoogleCalendarRequest,
  SaveNtfyRequest,
  SaveTelegramRequest,
  SaveTrelloRequest,
} from "@marshal/protocol";
import { ApiError } from "~/data/api-error";
import type { Ctx } from "~/mock/context";
import {
  applyIntegrationList,
  DISCORD_ID,
  GCAL_ID,
  GITHUB_ID,
  GMAIL_ID,
  NTFY_ID,
  TELEGRAM_ID,
  TRELLO_ID,
} from "./integrations";

/*
 * The writes the Settings Integrations screen makes on the daemon (section S29a). Every one of them
 * answers with the daemon's whole connection list, which is applied to the store as it arrives. One
 * table serves every connection kind (`docs/architecture.md` section 18), so the same three calls
 * serve GitHub today and the later phases' connections tomorrow.
 *
 * The words belong to the screen: "GitHub connected" is said there. A refusal is already shown by
 * `optimistic` in the daemon's own words, and the store then keeps what it had.
 */

const DETECT_FAILED = "Marshal could not look for your chat. Try again.";

/** Runs one connection write and applies the whole list it answers. False means the daemon refused it. */
async function write(
  c: Ctx,
  key: string,
  request: () => Promise<IntegrationList>,
): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  try {
    const list = await c.optimistic({
      key,
      apply: () => undefined,
      request,
      rollback: () => undefined,
    });
    applyIntegrationList(c, list);
    return true;
  } catch {
    return false;
  }
}

/**
 * Stores the GitHub App's connection: the App's id, its installation id, its private key, and its
 * webhook secret, which arrive together because no part of it is any use alone. The daemon validates
 * the key, writes it and the secret to the OS keychain, tests the connection, and answers the whole
 * list, so nothing is judged here.
 */
export async function connectGitHub(c: Ctx, body: SaveGitHubRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${GITHUB_ID}`, () => api.saveIntegration(GITHUB_ID, body));
}

/** Stores the Trello connection: the key, the token, the board, and the webhook secret's pair. */
export async function connectTrello(c: Ctx, body: SaveTrelloRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${TRELLO_ID}`, () => api.saveIntegration(TRELLO_ID, body));
}

/**
 * Stores the Telegram bot connection: the bot's token and the chat notices go to. The daemon writes
 * the token to the keychain, tests the bot against Telegram, and answers the whole list.
 */
export async function connectTelegram(c: Ctx, body: SaveTelegramRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${TELEGRAM_ID}`, () => api.saveIntegration(TELEGRAM_ID, body));
}

/**
 * Finds the chat that most recently wrote to the bot, from its token alone, so the connection can be
 * saved without looking up a numeric id. It saves nothing and sends nothing. A refusal comes back as
 * the daemon's own sentence, for the form to show beside the field it is about.
 */
export async function detectTelegramChat(
  c: Ctx,
  body: DetectTelegramChatRequest,
): Promise<DetectTelegramChatAnswer | { error: string }> {
  const api = c.env.data?.api;
  if (!api) return { error: "Marshal is not connected to its daemon." };
  try {
    return await api.detectTelegramChat(body);
  } catch (error) {
    return { error: error instanceof ApiError ? error.message : DETECT_FAILED };
  }
}

/**
 * Stores the Discord bot connection: the bot's token and the channel notices go to. The daemon
 * writes the token to the keychain, tests the bot against Discord, and answers the whole list.
 */
export async function connectDiscord(c: Ctx, body: SaveDiscordRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${DISCORD_ID}`, () => api.saveIntegration(DISCORD_ID, body));
}

/**
 * Stores the Google Calendar OAuth client. This alone does not connect anything: the row stays
 * "Not connected" until the owner opens `authorizeGoogleCalendar`'s URL and grants access.
 */
export async function connectGoogleCalendar(
  c: Ctx,
  body: SaveGoogleCalendarRequest,
): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${GCAL_ID}`, () => api.saveIntegration(GCAL_ID, body));
}

/**
 * Stores which Gmail label to watch and which project a labeled email becomes a card in. It has
 * no client of its own: Google Calendar's own consent, once granted, covers Gmail too.
 */
export async function connectGmail(c: Ctx, body: SaveGmailRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${GMAIL_ID}`, () => api.saveIntegration(GMAIL_ID, body));
}

/** The consent URL for Google Calendar's OAuth flow, for the owner's own browser to open. */
export async function authorizeGoogleCalendar(c: Ctx): Promise<AuthorizeURL | null> {
  const api = c.env.data?.api;
  if (!api) return null;
  try {
    return await api.authorizeGoogleCalendar();
  } catch {
    return null;
  }
}

/** Forgets a connection's settings and its secret. One that was never set up is not an error. */
export async function disconnectIntegration(c: Ctx, id: string): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration-del:${id}`, () => api.removeIntegration(id));
}

/**
 * Runs one connection's test now and mirrors what it found. The answer is the test's own result, so
 * a passed check list comes back true and one with a failed check, which the daemon answered as a
 * result rather than an error, comes back false and is said in the screen's own words. A test that
 * could not run - including one asked for inside the daemon's cooldown - throws, and its own
 * sentence is shown by `optimistic`.
 *
 * After a test the list is read again, because a test can change a row's state (an error after a
 * failed check) and only the daemon decides that. The re-read is best effort: the test's own result
 * is already on the row if it fails.
 */
export async function testIntegration(c: Ctx, id: string): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  let ok: boolean;
  try {
    const result = await c.optimistic({
      key: `integration-test:${id}`,
      apply: () => undefined,
      request: () => api.testIntegration(id),
      rollback: () => undefined,
    });
    ok = result.ok;
  } catch {
    return false;
  }
  try {
    applyIntegrationList(c, await api.listIntegrations());
  } catch {
    // The test ran; only the re-read failed, so the row keeps the state it had.
  }
  return ok;
}

/**
 * Stores the ntfy connection: the topic notices are published to, and optionally the server and an
 * access token. The daemon writes the token to the keychain, publishes a test message, and answers
 * the whole list.
 */
export async function connectNtfy(c: Ctx, body: SaveNtfyRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${NTFY_ID}`, () => api.saveIntegration(NTFY_ID, body));
}
