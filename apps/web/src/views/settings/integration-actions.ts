import type {
  AuthorizeURL,
  SaveDiscordRequest,
  SaveGitHubRequest,
  SaveGmailRequest,
  SaveGoogleCalendarRequest,
  SaveTelegramRequest,
  SaveTrelloRequest,
} from "@marshal/protocol";
import { M } from "~/mock";

/**
 * The daemon-side buttons of a connection row in Settings (section S29a). The words belong here,
 * with the screen: the daemon's own refusal is already shown by `optimistic` in its own words, so
 * nothing here judges one. Each answers whether it worked, so the form can close on a save.
 */

/** Stores the GitHub App's connection and says so. The daemon tests it as part of the same call. */
export function connectGitHub(body: SaveGitHubRequest): Promise<boolean> {
  return M.connectGitHub(body).then((saved) => {
    if (saved) M.toast("GitHub connected");
    return saved;
  });
}

/** Stores the Trello connection and says so. The daemon tests it as part of the same call. */
export function connectTrello(body: SaveTrelloRequest): Promise<boolean> {
  return M.connectTrello(body).then((saved) => {
    if (saved) M.toast("Trello connected");
    return saved;
  });
}

/** Stores the Telegram bot connection and says so. The daemon tests it as part of the same call. */
export function connectTelegram(body: SaveTelegramRequest): Promise<boolean> {
  return M.connectTelegram(body).then((saved) => {
    if (saved) M.toast("Telegram connected");
    return saved;
  });
}

/** Stores the Discord bot connection and says so. The daemon tests it as part of the same call. */
export function connectDiscord(body: SaveDiscordRequest): Promise<boolean> {
  return M.connectDiscord(body).then((saved) => {
    if (saved) M.toast("Discord connected");
    return saved;
  });
}

/** Stores which Gmail label to watch and says so. Sharing Google Calendar's own access. */
export function connectGmail(body: SaveGmailRequest): Promise<boolean> {
  return M.connectGmail(body).then((saved) => {
    if (saved) M.toast("Gmail connected");
    return saved;
  });
}

/** Stores the Google Calendar OAuth client. Granting access is a separate step. */
export function connectGoogleCalendar(body: SaveGoogleCalendarRequest): Promise<boolean> {
  return M.connectGoogleCalendar(body).then((saved) => {
    if (saved) M.toast("Google Calendar's client is saved. Grant access to finish connecting.");
    return saved;
  });
}

/** The consent URL for Google Calendar's OAuth flow. */
export function authorizeGoogleCalendar(): Promise<AuthorizeURL | null> {
  return M.authorizeGoogleCalendar();
}

/** Runs a connection's own test now and says what it found. */
export function testConnection(id: string, name: string): Promise<void> {
  return M.testIntegration(id).then((ok) => {
    M.toast(ok ? `${name} test passed` : `${name} test failed`);
  });
}

/**
 * Forgets a connection's settings and its secret, after asking. The confirm copy says what is
 * discarded rather than promising a backup: the daemon keeps none.
 */
export function disconnectConnection(id: string, name: string): void {
  M.confirm({
    title: `Disconnect ${name}`,
    message: `This forgets ${name}'s settings and its secret. Marshal stops using it until you connect it again.`,
    action: "Disconnect",
    destructive: true,
    run: () => {
      void M.disconnectIntegration(id).then((done) => {
        if (done) M.toast(`${name} disconnected`);
      });
    },
  });
}
