import type { IntegrationList } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { type IntegrationState, toIntegrationStates } from "~/data/mappers/integrations";
import { isDaemon, type SectionId } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import type { Syncer } from "./syncer";

/*
 * The connections Marshal is set up with (section S29a, docs/architecture.md sections 11.1 and 18,
 * build-plan 6.11): today the GitHub row in Settings, and in later phases Trello, a calendar, Gmail,
 * Telegram, Discord, and an Obsidian vault, each its own section (S29b to S29g).
 *
 * The daemon lists every connection it knows, whether or not it is set up, and answers the whole
 * list to a read, a save, and a remove alike. This syncer takes the connections whose own section is
 * switched and writes the daemon's half of each row - its status, its sentence, and its last test -
 * onto the row the store already has, keeping the app's own words (the name and the icon). A
 * connection whose section is still the mock's is left exactly as the seed made it.
 *
 * There are no connection events: every route answers the whole list, so a load, a save, a remove,
 * and a test's re-read all arrive through `applyIntegrationList`. It publishes no topic, so it is
 * loaded once when the app comes online and after every re-sync.
 */

/** Which section owns each connection the daemon lists. */
const CONNECTION_SECTIONS: Readonly<Record<string, SectionId>> = {
  github: "S29a",
  obsidian: "S29b",
  trello: "S29c",
  gcal: "S29d",
  gmail: "S29e",
  telegram: "S29f",
  discord: "S29g",
};

/** The GitHub App connection's own id, the one its row, its keychain entry, and its test are filed under. */
export const GITHUB_ID = "github";

/**
 * The Obsidian vault connection's own id (section S29b). Marshal owns this one - the vault is its
 * own data folder, not a setting a person fills in - so it has a test the same as GitHub's does,
 * but no save and no keychain entry.
 */
export const OBSIDIAN_ID = "obsidian";

/** The Trello connection's own id (section S29c). */
export const TRELLO_ID = "trello";

/** The Google Calendar connection's own id (section S29d). */
export const GCAL_ID = "gcal";

/** The Gmail connection's own id (section S29e). */
export const GMAIL_ID = "gmail";

/** The Telegram connection's own id (section S29f). */
export const TELEGRAM_ID = "telegram";

/** The Discord connection's own id (section S29g). */
export const DISCORD_ID = "discord";

export const integrationsSyncer: Syncer<IntegrationState[]> = {
  section: "S29a",
  topics: [],
  async load(api: ApiClient) {
    return toIntegrationStates(await api.listIntegrations());
  },
  apply(ctx, states) {
    applyIntegrationStates(ctx, states);
  },
};

/** The daemon's list answer, applied to the store. Every connection route answers this shape. */
export function applyIntegrationList(ctx: Ctx, list: IntegrationList): void {
  applyIntegrationStates(ctx, toIntegrationStates(list));
}

/**
 * Writes the daemon's half of each connection whose section is switched onto the store's own row,
 * matched by id. The app's words (name and icon) and the connections of later phases are untouched,
 * so a store with one connection on the daemon and the rest on the mock is exactly as it reads.
 */
export function applyIntegrationStates(ctx: Ctx, states: readonly IntegrationState[]): void {
  const table = sectionsOf(ctx.env);
  for (const state of states) {
    const section = CONNECTION_SECTIONS[state.id];
    if (!section || !isDaemon(section, table)) continue;
    const row = ctx.S.integrations.find((integration) => integration.id === state.id);
    if (!row) continue;
    row.st = state.st;
    row.detail = state.detail;
    row.lastTest = state.lastTest;
  }
}

/** True when the daemon owns this connection's row: its own section is switched. */
export function connectionOnDaemon(ctx: Ctx, id: string): boolean {
  const section = CONNECTION_SECTIONS[id];
  return section !== undefined && isDaemon(section, sectionsOf(ctx.env));
}
