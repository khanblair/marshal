import type { IdCounters } from "../ids";
import type { CalEvent, Integration, Profile, Provider, Role, Schedule } from "../settings-types";
import type { Activity, Card, Chat, Check, FeedItem, Msg, Notice, Person, Project } from "../types";
import { seedCalEvents, seedSchedules } from "./calendar";
import { seedCardChats } from "./card-chats";
import { PEOPLE, seedCardExtras } from "./card-extras";
import { seedCards } from "./cards";
import { checksFor } from "./checks";
import { activityFrom } from "./generic";
import { seedFeed, seedNotices } from "./home";
import { createMsgFactory } from "./messages";
import { seedProjectChats } from "./project-chats";
import { seedProfile, seedProjects } from "./projects";
import { seedIntegrations, seedProviders, seedRoles } from "./settings";

export interface Seed {
  people: Person[];
  projects: Project[];
  cards: Card[];
  chat: Record<number, Msg[]>;
  chats: Record<string, Chat[]>;
  act: Record<number, Activity[]>;
  checks: Record<number, Check[]>;
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
  const act: Record<number, Activity[]> = {};
  const checks: Record<number, Check[]> = {};
  for (const c of cards) act[c.id] = activityFrom(chat[c.id] ?? [], c.upd, ids);
  for (const c of cards) checks[c.id] = checksFor(c);
  return {
    people: structuredClone(PEOPLE),
    projects: seedProjects(),
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
