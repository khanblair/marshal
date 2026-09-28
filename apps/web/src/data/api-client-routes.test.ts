import type {
  ChatListSnapshot,
  CreateSavedViewRequest,
  LabelSnapshot,
  Preferences,
  Profile,
  Progress,
  SavedView,
  SavedViewListSnapshot,
  SearchSnapshot,
  UserListSnapshot,
} from "@marshal/protocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { card, chat, failureOf, make, project, TOKEN } from "./api-client-test-helpers";
import { emptyAnswer, errorAnswer, hang, jsonAnswer } from "./testing/fake-fetch";
import { golden } from "./testing/golden";

const labelSnapshot = golden<LabelSnapshot>("label-snapshot");
const label = labelSnapshot.labels[0]!;
const chatList = golden<ChatListSnapshot>("chat-list");

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("one typed method for each route", () => {
  const board = golden("board");
  const agents = golden("agents");
  const routes = {
    "GET /v1/health": () => jsonAnswer(golden("health")),
    "GET /v1/auth/whoami": () => jsonAnswer(golden("whoami")),
    "GET /v1/projects": () => jsonAnswer(golden("project-list")),
    "POST /v1/projects": () => jsonAnswer(project, 201),
    "GET /v1/projects/web-dashboard": () => jsonAnswer(project),
    "PATCH /v1/projects/web-dashboard": () => jsonAnswer(project),
    "DELETE /v1/projects/web-dashboard": () => emptyAnswer(),
    "GET /v1/projects/web-dashboard/board": () => jsonAnswer(board),
    "POST /v1/projects/web-dashboard/cards": () => jsonAnswer(card, 201),
    [`GET /v1/cards/${card.id}`]: () => jsonAnswer(card),
    [`POST /v1/cards/${card.id}/start`]: () => jsonAnswer(card),
    [`POST /v1/cards/${card.id}/messages`]: () => emptyAnswer(),
    [`POST /v1/cards/${card.id}/stop`]: () => emptyAnswer(),
    [`POST /v1/cards/${card.id}/resume`]: () => emptyAnswer(),
    "GET /v1/agents": () => jsonAnswer(agents),
    "POST /v1/agents/refresh": () => jsonAnswer(agents),
    [`POST /v1/cards/${card.id}/move`]: () => jsonAnswer(card),
    [`PATCH /v1/cards/${card.id}`]: () => jsonAnswer(card),
    [`DELETE /v1/cards/${card.id}`]: () => emptyAnswer(),
    [`POST /v1/cards/${card.id}/fork`]: () => jsonAnswer(card, 201),
    [`POST /v1/cards/${card.id}/ci-failure`]: () =>
      jsonAnswer(golden("simulate-ci-failure-result")),
    "GET /v1/projects/web-dashboard/labels": () => jsonAnswer(labelSnapshot),
    "POST /v1/projects/web-dashboard/labels": () => jsonAnswer(label, 201),
    [`PATCH /v1/labels/${label.id}`]: () => jsonAnswer(label),
    [`DELETE /v1/labels/${label.id}`]: () => emptyAnswer(),
    "GET /v1/home/dashboard": () => jsonAnswer(golden("home-snapshot")),
    "GET /v1/home/activity": () => jsonAnswer(golden("home-activity")),
    [`GET /v1/cards/${card.id}/messages`]: () => jsonAnswer(golden("chat-messages")),
    [`GET /v1/cards/${card.id}/activity`]: () => jsonAnswer(golden("activity-items")),
    [`GET /v1/cards/${card.id}/messages/01M3EFYNXW6KZKMD6QY52DPSPJ`]: () =>
      jsonAnswer(golden("chat-message-detail")),
    "GET /v1/projects/web-dashboard/chats": () => jsonAnswer(chatList),
    "GET /v1/projects/web-dashboard/chats?archived=true": () => jsonAnswer(chatList),
    "POST /v1/projects/web-dashboard/chats": () => jsonAnswer(chat, 201),
    [`PATCH /v1/chats/${chat.id}`]: () => jsonAnswer(chat),
    [`POST /v1/chats/${chat.id}/archive`]: () => jsonAnswer(chat),
    [`POST /v1/chats/${chat.id}/restore`]: () => jsonAnswer(chat),
    [`DELETE /v1/chats/${chat.id}`]: () => emptyAnswer(),
    [`POST /v1/chats/${chat.id}/messages`]: () => emptyAnswer(),
    [`GET /v1/chats/${chat.id}/messages`]: () => jsonAnswer(golden("chat-messages")),
    [`GET /v1/chats/${chat.id}/messages/01M3EFYNXW6KZKMD6QY52DPSPJ`]: () =>
      jsonAnswer(golden("chat-message-detail")),
  };

  it("answers with what the daemon sent, or nothing for a 204", async () => {
    const { client, calls } = make(routes);
    expect(await client.health()).toEqual(golden("health"));
    expect(await client.whoami()).toEqual(golden("whoami"));
    expect(await client.listProjects()).toEqual(golden("project-list"));
    expect(await client.createProject({ source: "folder", path: "/x" })).toEqual(project);
    expect(await client.getProject("web-dashboard")).toEqual(project);
    expect(await client.updateProject("web-dashboard", { name: "New" })).toEqual(project);
    expect(await client.removeProject("web-dashboard")).toBeUndefined();
    expect(await client.board("web-dashboard")).toEqual(board);
    expect(await client.createCard("web-dashboard", { title: "A card" })).toEqual(card);
    expect(await client.getCard(card.id)).toEqual(card);
    expect(await client.startCard(card.id)).toEqual(card);
    expect(await client.sendMessage(card.id, { text: "Hello" })).toBeUndefined();
    expect(await client.stopCard(card.id)).toBeUndefined();
    expect(await client.resumeCard(card.id)).toBeUndefined();
    expect(await client.agents()).toEqual(agents);
    expect(await client.refreshAgents()).toEqual(agents);
    expect(await client.moveCard(card.id, { state: "review" })).toEqual(card);
    expect(await client.updateCard(card.id, { title: "New" })).toEqual(card);
    expect(await client.removeCard(card.id)).toBeUndefined();
    expect(await client.forkCard(card.id)).toEqual(card);
    expect(await client.simulateCIFailure(card.id, { mode: "synthetic" })).toEqual(
      golden("simulate-ci-failure-result"),
    );
    expect(await client.listLabels("web-dashboard")).toEqual(labelSnapshot);
    expect(await client.createLabel("web-dashboard", { name: "auth" })).toEqual(label);
    expect(await client.updateLabel(label.id, { color: "red" })).toEqual(label);
    expect(await client.removeLabel(label.id)).toBeUndefined();
    expect(await client.home()).toEqual(golden("home-snapshot"));
    expect(await client.homeActivity()).toEqual(golden("home-activity"));
    expect(await client.messages(card.id)).toEqual(golden("chat-messages"));
    expect(await client.activity(card.id)).toEqual(golden("activity-items"));
    expect(await client.messageDetail(card.id, "01M3EFYNXW6KZKMD6QY52DPSPJ")).toEqual(
      golden("chat-message-detail"),
    );
    expect(await client.listChats("web-dashboard")).toEqual(chatList);
    expect(await client.listChats("web-dashboard", true)).toEqual(chatList);
    expect(await client.createChat("web-dashboard", { title: "Plan" })).toEqual(chat);
    expect(await client.updateChat(chat.id, { title: "New" })).toEqual(chat);
    expect(await client.archiveChat(chat.id)).toEqual(chat);
    expect(await client.restoreChat(chat.id)).toEqual(chat);
    expect(await client.deleteChat(chat.id)).toBeUndefined();
    expect(await client.sendChatMessage(chat.id, { text: "Hello" })).toBeUndefined();
    expect(await client.chatMessages(chat.id)).toEqual(golden("chat-messages"));
    expect(await client.chatMessageDetail(chat.id, "01M3EFYNXW6KZKMD6QY52DPSPJ")).toEqual(
      golden("chat-message-detail"),
    );
    expect(calls.map((c) => `${c.method} ${c.url}`)).toEqual(Object.keys(routes));
  });

  it("sends a body for remove only when one is given", async () => {
    const { client, calls } = make(routes);
    await client.removeProject("web-dashboard");
    await client.removeProject("web-dashboard", { keepBranches: true, keepMemory: false });
    expect(calls[0]?.body).toBeNull();
    expect(calls[0]?.headers["content-type"]).toBeUndefined();
    expect(JSON.parse(calls[1]?.body ?? "")).toEqual({ keepBranches: true, keepMemory: false });
  });

  it("asks a list route for the size and the cursor it was given, and nothing it was not", async () => {
    const page = golden("chat-messages");
    const { client, calls } = make({
      ...routes,
      [`GET /v1/cards/${card.id}/messages?limit=20&cursor=abc`]: () => jsonAnswer(page),
      [`GET /v1/cards/${card.id}/activity?limit=5&kind=file`]: () => jsonAnswer(page),
    });
    await client.messages(card.id);
    await client.messages(card.id, { limit: 20, cursor: "abc" });
    await client.activity(card.id, { kind: "file", limit: 5 });
    expect(calls[0]?.url).toBe(`/v1/cards/${card.id}/messages`);
    expect(calls[1]?.url).toBe(`/v1/cards/${card.id}/messages?limit=20&cursor=abc`);
    expect(calls[2]?.url).toBe(`/v1/cards/${card.id}/activity?limit=5&kind=file`);
  });

  it("asks the Home dashboard for its chart range only when one was chosen", async () => {
    const snapshot = golden("home-snapshot");
    const { client, calls } = make({
      "GET /v1/home/dashboard": () => jsonAnswer(snapshot),
      "GET /v1/home/dashboard?range=30": () => jsonAnswer(snapshot),
    });
    await client.home();
    await client.home({ range: 30 });
    expect(calls[0]?.url).toBe("/v1/home/dashboard");
    expect(calls[1]?.url).toBe("/v1/home/dashboard?range=30");
  });

  it("asks the Home activity list for its filters and paging, and nothing else", async () => {
    const page = golden("home-activity");
    const { client, calls } = make({
      "GET /v1/home/activity": () => jsonAnswer(page),
      "GET /v1/home/activity?limit=5&cursor=abc&kind=merge&project=web": () => jsonAnswer(page),
    });
    await client.homeActivity();
    await client.homeActivity({ limit: 5, cursor: "abc", kind: "merge", project: "web" });
    expect(calls[0]?.url).toBe("/v1/home/activity");
    expect(calls[1]?.url).toBe("/v1/home/activity?limit=5&cursor=abc&kind=merge&project=web");
  });

  it("sends a chat's message as JSON, and asks its history the way a card's is asked", async () => {
    const page = golden("chat-messages");
    const { client, calls } = make({
      [`POST /v1/chats/${chat.id}/messages`]: () => emptyAnswer(),
      [`GET /v1/chats/${chat.id}/messages`]: () => jsonAnswer(page),
      [`GET /v1/chats/${chat.id}/messages?limit=20&cursor=abc`]: () => jsonAnswer(page),
    });
    await client.sendChatMessage(chat.id, { text: "What is blocked?" });
    await client.chatMessages(chat.id);
    await client.chatMessages(chat.id, { limit: 20, cursor: "abc" });
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual({ text: "What is blocked?" });
    expect(calls[0]?.headers["content-type"]).toBe("application/json");
    expect(calls[1]?.url).toBe(`/v1/chats/${chat.id}/messages`);
    expect(calls[2]?.url).toBe(`/v1/chats/${chat.id}/messages?limit=20&cursor=abc`);
  });

  it("sends no body for the card commands that take none", async () => {
    const { client, calls } = make(routes);
    await client.startCard(card.id);
    await client.stopCard(card.id);
    expect(calls.every((c) => c.body === null)).toBe(true);
  });

  it("asks for a simulated CI failure on the card's own route, with the mode it was given", async () => {
    const result = golden("simulate-ci-failure-result");
    const { client, calls } = make({
      [`POST /v1/cards/${card.id}/ci-failure`]: () => jsonAnswer(result),
    });
    expect(await client.simulateCIFailure(card.id, { mode: "real" })).toEqual(result);
    expect(calls[0]?.method).toBe("POST");
    expect(calls[0]?.url).toBe(`/v1/cards/${card.id}/ci-failure`);
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual({ mode: "real" });
  });

  it("has a generic request for a route that has no method yet", async () => {
    const { client } = make({ "GET /v1/other": () => jsonAnswer({ ok: true }) });
    expect(await client.request<{ ok: boolean }>("GET", "/v1/other")).toEqual({ ok: true });
  });
});

describe("search", () => {
  const found = golden<SearchSnapshot>("search");

  it("asks for the query it was given and answers with what the daemon sent", async () => {
    const { client, calls } = make({ "GET /v1/search?q=token": () => jsonAnswer(found) });
    expect(await client.search("token")).toEqual(found);
    expect(calls[0]?.method).toBe("GET");
    expect(calls[0]?.url).toBe("/v1/search?q=token");
    expect(calls[0]?.body).toBeNull();
    expect(calls[0]?.headers.authorization).toBe(`Bearer ${TOKEN}`);
  });

  it("escapes the query, so a card number's # and a space reach the daemon whole", async () => {
    const { client, calls } = make({
      "GET /v1/search?q=api%2341": () => jsonAnswer(found),
      "GET /v1/search?q=rate+limiting+%26+%C3%A9": () => jsonAnswer(found),
    });
    await client.search("api#41");
    await client.search("rate limiting & é");
    expect(calls.map((call) => call.url)).toEqual([
      "/v1/search?q=api%2341",
      "/v1/search?q=rate+limiting+%26+%C3%A9",
    ]);
  });

  it("asks for an empty query as it is, and reads the empty answer", async () => {
    const empty = golden<SearchSnapshot>("search-empty");
    const { client, calls } = make({ "GET /v1/search?q=": () => jsonAnswer(empty) });
    expect(await client.search("")).toEqual(empty);
    expect(calls[0]?.url).toBe("/v1/search?q=");
  });

  it("throws the daemon's own sentence when it refuses the query", async () => {
    const { client } = make({
      "GET /v1/search?q=x": () =>
        errorAnswer(400, "invalid_argument", "Search for 200 characters or fewer."),
    });
    const error = await failureOf(client.search("x"));
    expect(error.code).toBe("invalid_argument");
    expect(error.status).toBe(400);
    expect(error.message).toBe("Search for 200 characters or fewer.");
  });

  it("can be cancelled, which fails the call with the code aborted", async () => {
    const { client } = make({ "GET /v1/search?q=x": () => hang() });
    const controller = new AbortController();
    const pending = failureOf(client.search("x", { signal: controller.signal }));
    controller.abort();
    expect((await pending).code).toBe("aborted");
  });
});

describe("the person", () => {
  const profile = golden<Profile>("profile");
  const users = golden<UserListSnapshot>("user-list");
  const progress = golden<Progress>("progress");
  const preferences = golden<Preferences>("preferences");

  it("reads the profile, the users, the progress, and the preferences", async () => {
    const { client, calls } = make({
      "GET /v1/me": () => jsonAnswer(profile),
      "GET /v1/users": () => jsonAnswer(users),
      "GET /v1/me/progress": () => jsonAnswer(progress),
      "GET /v1/me/preferences": () => jsonAnswer(preferences),
    });
    expect(await client.me()).toEqual(profile);
    expect(await client.users()).toEqual(users);
    expect(await client.progress()).toEqual(progress);
    expect(await client.preferences()).toEqual(preferences);
    expect(calls.every((call) => call.body === null)).toBe(true);
  });

  it("sends only the fields it is given to PATCH /v1/me, as JSON", async () => {
    const { client, calls } = make({ "PATCH /v1/me": () => jsonAnswer(profile) });
    expect(await client.updateMe({ name: "Ada Okafor", email: "" })).toEqual(profile);
    expect(calls[0]?.headers["content-type"]).toBe("application/json");
    expect(JSON.parse(calls[0]?.body ?? "null")).toEqual({ name: "Ada Okafor", email: "" });
  });

  it("throws the daemon's own sentence when it refuses a profile", async () => {
    const { client } = make({
      "PATCH /v1/me": () =>
        errorAnswer(
          400,
          "invalid_argument",
          "Marshal does not know that time zone. Choose one from the list.",
        ),
    });
    const error = await failureOf(client.updateMe({ timeZone: "Europe/Atlantis" }));
    expect(error.message).toBe("Marshal does not know that time zone. Choose one from the list.");
    expect(error.status).toBe(400);
  });

  it("uploads an avatar as the raw image, with the image's own type and not as JSON", async () => {
    const { client, calls } = make({ "POST /v1/me/avatar": () => jsonAnswer(profile) });
    const image = new Blob([new Uint8Array([137, 80, 78, 71])], { type: "image/png" });
    expect(await client.uploadAvatar(image)).toEqual(profile);
    expect(calls[0]?.method).toBe("POST");
    expect(calls[0]?.headers["content-type"]).toBe("image/png");
    expect(calls[0]?.headers.authorization).toBe(`Bearer ${TOKEN}`);
    expect(calls[0]?.body).toBeNull();
    expect(calls[0]?.raw).toBe(image);
  });

  it("throws the daemon's sentence when an image is too large or not an image it takes", async () => {
    const { client } = make({
      "POST /v1/me/avatar": () =>
        errorAnswer(
          400,
          "invalid_argument",
          "That image is larger than 2 MB. Choose a smaller one.",
        ),
    });
    const image = new Blob(["x"], { type: "image/png" });
    expect((await failureOf(client.uploadAvatar(image))).message).toBe(
      "That image is larger than 2 MB. Choose a smaller one.",
    );
  });

  it("removes an avatar and answers with the profile", async () => {
    const { client, calls } = make({ "DELETE /v1/me/avatar": () => jsonAnswer(profile) });
    expect(await client.removeAvatar()).toEqual(profile);
    expect(calls[0]?.body).toBeNull();
  });

  it("fetches an avatar image with the token and hands back its bytes", async () => {
    const path = "/v1/users/01M3C107JB041061050R3GG2U1/avatar?v=1759233600000";
    const { client, calls } = make({
      [`GET ${path}`]: () =>
        new Response(new Uint8Array([1, 2, 3]), { headers: { "Content-Type": "image/png" } }),
    });
    const image = await client.avatarImage(path);
    expect(image.size).toBe(3);
    expect(image.type).toBe("image/png");
    expect(calls[0]?.headers.authorization).toBe(`Bearer ${TOKEN}`);
    expect(calls[0]?.headers.accept).toBe("image/*");
  });

  it("throws the daemon's error when there is no avatar, as text and not as bytes", async () => {
    const { client } = make({
      "GET /v1/users/u1/avatar": () =>
        errorAnswer(404, "not_found", "Marshal cannot find that avatar. It may have been removed."),
    });
    const error = await failureOf(client.avatarImage("/v1/users/u1/avatar"));
    expect(error.code).toBe("not_found");
    expect(error.message).toBe("Marshal cannot find that avatar. It may have been removed.");
  });

  it("sends a preferences change as JSON and answers with the whole preferences", async () => {
    const { client, calls } = make({ "PATCH /v1/me/preferences": () => jsonAnswer(preferences) });
    const change = {
      theme: "dark",
      projects: { api: { swimlane: "role", savedViewId: "" } },
    } as const;
    expect(await client.updatePreferences(change)).toEqual(preferences);
    expect(JSON.parse(calls[0]?.body ?? "null")).toEqual(change);
  });

  it("changes the progress and resets the first launch", async () => {
    const { client, calls } = make({
      "PATCH /v1/me/progress": () => jsonAnswer(progress),
      "POST /v1/dev/reset-first-launch": () => jsonAnswer(progress),
    });
    await client.updateProgress({ tutorial: { status: "skipped" } });
    await client.resetFirstLaunch();
    expect(JSON.parse(calls[0]?.body ?? "null")).toEqual({ tutorial: { status: "skipped" } });
    expect(calls[1]?.body).toBeNull();
  });

  it("says nothing exists at the dev reset on a daemon that is not in dev mode", async () => {
    const { client } = make({
      "POST /v1/dev/reset-first-launch": () =>
        errorAnswer(404, "not_found", "Marshal has nothing at that address."),
    });
    expect((await failureOf(client.resetFirstLaunch())).status).toBe(404);
  });
});

describe("saved views", () => {
  const list = golden<SavedViewListSnapshot>("saved-view-list");
  const view = golden<SavedView>("saved-view");

  it("lists a project's saved views", async () => {
    const { client } = make({ "GET /v1/projects/api/saved-views": () => jsonAnswer(list) });
    expect(await client.listSavedViews("api")).toEqual(list);
  });

  it("saves a view, on a 201 for a new one and on a 200 for a name that was used", async () => {
    const { client, calls } = make({
      "POST /v1/projects/api/saved-views": (() => {
        let answers = 0;
        return () => jsonAnswer(view, answers++ === 0 ? 201 : 200);
      })(),
    });
    const body: CreateSavedViewRequest = {
      name: "Needs me",
      filters: [{ key: "status", value: "needs" }],
      swimlane: "none",
    };
    expect(await client.createSavedView("api", body)).toEqual(view);
    expect(await client.createSavedView("api", body)).toEqual(view);
    expect(JSON.parse(calls[0]?.body ?? "null")).toEqual(body);
  });

  it("renames and removes a view by its id, and escapes the id", async () => {
    const { client, calls } = make({
      "PATCH /v1/saved-views/a%2Fb": () => jsonAnswer(view),
      "DELETE /v1/saved-views/a%2Fb": () => emptyAnswer(),
    });
    await client.updateSavedView("a/b", { name: "Mine" });
    await client.removeSavedView("a/b");
    expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
      "PATCH /v1/saved-views/a%2Fb",
      "DELETE /v1/saved-views/a%2Fb",
    ]);
  });

  it("throws the daemon's sentence when a name is used or the limit is reached", async () => {
    const { client } = make({
      "POST /v1/projects/api/saved-views": () =>
        errorAnswer(
          422,
          "invalid_argument",
          "A project can have at most 50 saved views. Delete one and try again.",
        ),
    });
    const error = await failureOf(client.createSavedView("api", { name: "x" }));
    expect(error.message).toBe(
      "A project can have at most 50 saved views. Delete one and try again.",
    );
  });
});
