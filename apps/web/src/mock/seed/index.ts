import type { CardKey } from "../card-key";
import type { IdCounters } from "../ids";
import type { CalEvent, Integration, Profile, Provider, Role, Schedule } from "../settings-types";
import type { Activity, Card, Chat, Check, FeedItem, Msg, Notice, Person } from "../types";
import { seedCalEvents, seedSchedules } from "./calendar";
import { seedCardChats } from "./card-chats";
import { PEOPLE, seedCardExtras } from "./card-extras";
import { seedCards } from "./cards";
import { checksFor } from "./checks";
import { activityFrom } from "./generic";
import { seedFeed, seedNotices } from "./home";
import { createMsgFactory } from "./messages";
import { seedProfile } from "./profile";
import { seedProjectChats } from "./project-chats";
import { seedIntegrations, seedProviders, seedRoles } from "./settings";

export interface Seed {
  people: Person[];
  cards: Card[];
  chat: Record<CardKey, Msg[]>;
  chats: Record<string, Chat[]>;
  act: Record<CardKey, Activity[]>;
  checks: Record<CardKey, Check[]>;
  roles: Role[];
  providers: Provider[];
  integrations: Integration[];
  schedules: Schedule[];
  calEvents: CalEvent[];
  notices: Notice[];
  feed: FeedItem[];
  profile: Profile;
}

/**
 * Builds fresh seed data. The steps run in the prototype's order because they share
 * the id counters: cards and their extras, card chats, project chats, activity, feed.
 */
export function buildSeed(ids: IdCounters, loadedAt: number): Seed {
  const b = createMsgFactory(ids);
  const cards = seedCards(loadedAt);
  seedCardExtras(cards, ids, loadedAt);
  const chat = seedCardChats(cards, b);
  const chats = seedProjectChats(b, ids, loadedAt);
  const act: Record<CardKey, Activity[]> = {};
  const checks: Record<CardKey, Check[]> = {};
  for (const c of cards) act[c.id] = activityFrom(chat[c.id] ?? [], c.upd, ids);
  for (const c of cards) checks[c.id] = checksFor(c);
  return {
    people: structuredClone(PEOPLE),
    cards,
    chat,
    chats,
    act,
    checks,
    roles: seedRoles(),
    providers: seedProviders(),
    integrations: seedIntegrations(),
    schedules: seedSchedules(),
    calEvents: seedCalEvents(),
    notices: seedNotices(loadedAt),
    feed: seedFeed(ids, loadedAt),
    profile: seedProfile(loadedAt),
  };
}
