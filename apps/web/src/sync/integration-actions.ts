import type {
  AuthorizeURL,
  DetectTelegramChatAnswer,
  DetectTelegramChatRequest,
  GitHubConnect,
  IntegrationList,
  SaveDiscordRequest,
  SaveGitHubTokenRequest,
  SaveGmailRequest,
  SaveGoogleCalendarRequest,
  SaveNtfyRequest,
  SaveTelegramRequest,
  SaveTrelloRequest,
} from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { type GitHubConnection, toGitHubConnection } from "~/data/mappers/integrations";
import { type ProviderTest, toProviderTest } from "~/data/mappers/providers";
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
 * serve GitHub today and the later phases' connections tomorrow. GitHub's sign-in routes answer the
 * sign-in's own state instead, and a refusal there is the daemon's sentence, not a toast.
 *
 * The words belong to the screen: "GitHub connected" is said there. A refusal is already shown by
 * `optimistic` in the daemon's own words, and the store then keeps what it had.
 */

const DETECT_FAILED = "Marshal could not look for your chat. Try again.";
const NO_DAEMON = "Marshal is not connected to its daemon.";
const GITHUB_FAILED = "Marshal could not reach GitHub. Try again.";
const TOKEN_FAILED = "Marshal could not check that token. Try again.";

/** What a GitHub sign-in call answers: the connection, or the daemon's own sentence for a refusal. */
export type GitHubAnswer = GitHubConnection | { error: string };

/** Asks one of the GitHub sign-in routes and narrows its answer. Nothing here is a toast. */
async function githubAnswer(
  c: Ctx,
  ask: (api: ApiClient) => Promise<GitHubConnect>,
): Promise<GitHubAnswer> {
  const api = c.env.data?.api;
  if (!api) return { error: NO_DAEMON };
  try {
    return toGitHubConnection(await ask(api));
  } catch (error) {
    return { error: error instanceof ApiError ? error.message : GITHUB_FAILED };
  }
}

/** Reads the connection list again. Best effort: the row keeps what it had when the read fails. */
async function refreshIntegrations(c: Ctx): Promise<void> {
  try {
    const api = c.env.data?.api;
    if (api) applyIntegrationList(c, await api.listIntegrations());
  } catch {
    // The list is read again with the next change.
  }
}

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
 * Starts a GitHub sign-in: the daemon asks GitHub for a code and answers it, pending. A refusal comes
 * back as the daemon's own sentence, for the dialog to show where the button was.
 */
export async function startGitHubConnect(c: Ctx): Promise<GitHubAnswer> {
  return githubAnswer(c, (api) => api.startGitHubConnect());
}

/**
 * Reads the GitHub sign-in, which also moves it on. It is polled, so a failed read is not a toast:
 * the dialog keeps what it had and asks again. When the answer says GitHub is connected, or not, and
 * the row says the opposite, the list is read again, because only that carries the row's sentence.
 */
export async function readGitHubConnect(c: Ctx): Promise<GitHubAnswer> {
  const answer = await githubAnswer(c, (api) => api.readGitHubConnect());
  if ("error" in answer) return answer;
  const row = c.S.integrations.find((integration) => integration.id === GITHUB_ID);
  const settled = answer.state === "connected" || answer.state === "idle";
  if (row && settled && (answer.state === "connected") !== (row.st !== "none")) {
    await refreshIntegrations(c);
  }
  return answer;
}

/** Cancels a pending GitHub sign-in. One that is not pending is not an error. */
export async function cancelGitHubConnect(c: Ctx): Promise<GitHubAnswer> {
  return githubAnswer(c, (api) => api.cancelGitHubConnect());
}

/**
 * Stores a GitHub personal access token, in place of whatever was connected. The daemon writes it to
 * the keychain, tests it, and answers the whole list, which is applied. A token GitHub refuses comes
 * back as the daemon's own sentence, for the dialog to show beside the field.
 */
export async function saveGitHubToken(
  c: Ctx,
  body: SaveGitHubTokenRequest,
): Promise<{ saved: true } | { error: string }> {
  const api = c.env.data?.api;
  if (!api) return { error: NO_DAEMON };
  try {
    applyIntegrationList(c, await api.saveGitHubToken(body));
    return { saved: true };
  } catch (error) {
    return { error: error instanceof ApiError ? error.message : TOKEN_FAILED };
  }
}

/** Tests a GitHub personal access token and saves nothing. */
export async function testGitHubToken(
  c: Ctx,
  body: SaveGitHubTokenRequest,
): Promise<ProviderTest | { error: string }> {
  const api = c.env.data?.api;
  if (!api) return { error: NO_DAEMON };
  try {
    return toProviderTest(await api.testGitHubToken(body));
  } catch (error) {
    return { error: error instanceof ApiError ? error.message : TOKEN_FAILED };
  }
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
