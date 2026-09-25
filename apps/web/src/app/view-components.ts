/** Which component draws each route, as the design's `<sc-if>` chain in `<main>` does. */
import type { Component } from "solid-js";
import { M, type ViewKey } from "~/mock";
import { AgentsView } from "~/views/agents/AgentsView";
import { BoardView } from "~/views/board/BoardView";
import { CalendarView } from "~/views/calendar/CalendarView";
import { ChatsView } from "~/views/chats/ChatsView";
import { HomeAllView } from "~/views/home/HomeAllView";
import { HomeView } from "~/views/home/HomeView";
import { ListView } from "~/views/list/ListView";
import { SettingsView } from "~/views/settings/SettingsView";
import { TimelineView } from "~/views/timeline/TimelineView";
import { isProject } from "./shell-layout";

/** The six views of a project, in the design's tab order. Split panes reuse them. */
export const VIEW_COMPONENTS: Record<ViewKey, Component> = {
  chat: ChatsView,
  agents: AgentsView,
  board: BoardView,
  list: ListView,
  timeline: TimelineView,
  calendar: CalendarView,
};

/** The component for the current route, or nothing for a project that no longer exists. */
export function routeComponent(): Component | undefined {
  const page = M.S.route.page;
  if (page === "home") return HomeView;
  if (page === "all") return HomeAllView;
  if (page === "settings") return SettingsView;
  return isProject() ? VIEW_COMPONENTS[M.S.route.view] : undefined;
}
