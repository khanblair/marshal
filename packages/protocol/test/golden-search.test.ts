import { describe, expect, it } from "vitest";
import {
  type CreateProjectRequest,
  MaxSearchQueryChars,
  ProjectSourceSample,
  ProjectSourceValues,
  SearchHitsPerKind,
  type SearchSnapshot,
} from "../src";
import { golden } from "./golden";

// Search and the sample project (docs/backend-checklist.md B2.11 and B2.12, N23 and N24). Each
// sample is checked against the generated type by the compiler, so a field that changes in Go
// stops this compiling until the sample matches the daemon's own file again.
describe("the search and sample project golden files", () => {
  it("has a search answer with a hit of every kind and the totals before the cut", () => {
    const sample: SearchSnapshot = {
      query: "token",
      projects: [
        {
          projectId: "token-service",
          name: "token-service",
          path: "/home/ada/code/token-service",
          language: "Go",
        },
      ],
      cards: [
        {
          cardId: "01M3C107JB041061050R3GG28A",
          key: "api#41",
          number: 41,
          title: "Refresh the token before it expires",
          state: "working",
          projectId: "api",
          projectName: "api-gateway",
        },
        {
          cardId: "01M3C107JB041061050R3GG28B",
          key: "web#7",
          number: 7,
          title: "Show the session timeout",
          state: "backlog",
          projectId: "web",
          projectName: "web-dashboard",
        },
      ],
      chats: [
        {
          chatId: "01M3C107JB041061050R3GG281",
          title: "Token rotation question",
          projectId: "api",
          projectName: "api-gateway",
          lastActiveAt: "2026-09-30T10:00:00.000Z",
        },
      ],
      totals: { projects: 1, cards: 11, chats: 1 },
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("search")).toEqual(sample);
    // The list was cut to the most hits of one kind, and the total says how many there were.
    expect(sample.cards.length).toBeLessThanOrEqual(SearchHitsPerKind);
    expect(sample.totals.cards).toBeGreaterThan(sample.cards.length);
  });

  it("sends three empty lists, never null, for an empty query", () => {
    const sample: SearchSnapshot = {
      query: "",
      projects: [],
      cards: [],
      chats: [],
      totals: { projects: 0, cards: 0, chats: 0 },
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("search-empty")).toEqual(sample);
  });

  it("has the limits the daemon enforces", () => {
    expect(SearchHitsPerKind).toBe(8);
    expect(MaxSearchQueryChars).toBe(200);
  });

  it("asks for the sample project with only its source", () => {
    const sample: CreateProjectRequest = { source: ProjectSourceSample };
    expect(golden("create-project-request-sample")).toEqual(sample);
    expect(ProjectSourceValues).toContain("sample");
  });
});
