import { type Card, M } from "~/mock";
import {
  ALL_LANE,
  addHint,
  addLabel,
  buildBoard,
  canQuickAdd,
  cardCountLabel,
  columnMinHeightClass,
  countByColumn,
  DONE_LIMIT,
  emptyText,
  laneKeyOf,
  laneKeys,
} from "./board-model";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const MOBILE_PACKAGES = [
  "apps/ios",
  "apps/android",
  "packages/ui",
  "packages/auth",
  "packages/api-client",
];

function makeCard(id: number, patch: Partial<Card> = {}): Card {
  const base = M.card(41);
  if (!base) throw new Error("seed card 41 missing");
  return { ...base, id, labels: [], pkg: null, ...patch };
}

describe("laneKeyOf", () => {
  const card = makeCard(1, {
    role: "Tester",
    agent: "Codex",
    pkg: "packages/ui",
    labels: ["ui", "bug"],
  });

  it.each([
    ["role", "Tester"],
    ["agent", "Codex"],
    ["package", "packages/ui"],
    ["label", "ui"],
    ["none", ALL_LANE],
    [undefined, ALL_LANE],
  ] as const)("keys a card by %s", (swim, key) => {
    expect(laneKeyOf(card, swim)).toBe(key);
  });

  it("names cards without a package or label", () => {
    const bare = makeCard(2);
    expect(laneKeyOf(bare, "package")).toBe("No package");
    expect(laneKeyOf(bare, "label")).toBe("No label");
  });
});

describe("laneKeys", () => {
  it("sorts by name with the No lanes last", () => {
    const list = [makeCard(1, { labels: ["ui"] }), makeCard(2), makeCard(3, { labels: ["auth"] })];
    expect(laneKeys(list, "label", [])).toEqual(["auth", "ui", "No label"]);
  });

  it("follows the project's package order and drops packages it does not list", () => {
    const list = [
      makeCard(1, { pkg: "packages/ui" }),
      makeCard(2, { pkg: "apps/ios" }),
      makeCard(3),
      makeCard(4, { pkg: "elsewhere" }),
    ];
    expect(laneKeys(list, "package", MOBILE_PACKAGES)).toEqual([
      "apps/ios",
      "packages/ui",
      "No package",
    ]);
  });

  it("falls back to one lane when there are no cards", () => {
    expect(laneKeys([], "role", [])).toEqual([ALL_LANE]);
    expect(laneKeys([], "package", MOBILE_PACKAGES)).toEqual([ALL_LANE]);
  });
});

describe("buildBoard", () => {
  const columns = M.COLUMNS;
  const base = { columns, showAllDone: false, packages: [], isCollapsed: () => false };

  it("builds one lane with all seven columns when there are no swimlanes", () => {
    const list = M.cardsOf("api");
    const board = buildBoard({ ...base, list, swim: "none" });
    expect(board.lanes).toHaveLength(1);
    const [lane] = board.lanes;
    expect(lane?.key).toBe(ALL_LANE);
    expect(lane?.count).toBe(list.length);
    expect(lane?.columns.map((c) => c.col)).toEqual([...columns]);
    expect(lane?.columns.find((c) => c.col === "working")?.total).toBe(2);
  });

  it("orders the cards of a column as the store does and mirrors them in the grid", () => {
    const board = buildBoard({ ...base, list: M.cardsOf("api"), swim: "none" });
    const working = board.lanes[0]?.columns.find((c) => c.col === "working");
    expect(working?.cards.map((c) => c.id)).toEqual([42, 41]);
    expect(board.grid[columns.indexOf("working")]).toEqual([42, 41]);
    expect(board.grid).toHaveLength(columns.length);
  });

  it("leaves a collapsed lane out of the columns and the grid", () => {
    const list = M.cardsOf("api");
    const board = buildBoard({
      ...base,
      list,
      swim: "role",
      isCollapsed: (key) => key === "Worker",
    });
    const worker = board.lanes.find((l) => l.key === "Worker");
    expect(worker?.collapsed).toBe(true);
    expect(worker?.columns).toEqual([]);
    expect(worker?.count).toBeGreaterThan(0);
    const ids = board.grid.flat();
    const workerIds = list.filter((c) => c.role === "Worker").map((c) => c.id);
    for (const id of workerIds) expect(ids).not.toContain(id);
  });

  it("limits Done to the newest 20 unless all are shown", () => {
    const done = Array.from({ length: DONE_LIMIT + 5 }, (_, i) =>
      makeCard(500 + i, { state: "done", upd: i }),
    );
    const limited = buildBoard({ ...base, list: done, swim: "none" });
    const column = limited.lanes[0]?.columns.find((c) => c.col === "done");
    expect(column?.cards).toHaveLength(DONE_LIMIT);
    expect(column?.total).toBe(DONE_LIMIT + 5);
    expect(column?.showAll).toBe(true);
    expect(column?.cards[0]?.id).toBe(500 + DONE_LIMIT + 4);
    const all = buildBoard({ ...base, list: done, swim: "none", showAllDone: true });
    const shown = all.lanes[0]?.columns.find((c) => c.col === "done");
    expect(shown?.cards).toHaveLength(DONE_LIMIT + 5);
    expect(shown?.showAll).toBe(false);
  });

  it("builds only the phone's column", () => {
    const board = buildBoard({ ...base, columns: ["needs"], list: M.cardsOf("api"), swim: "none" });
    expect(board.lanes[0]?.columns.map((c) => c.col)).toEqual(["needs"]);
    expect(board.grid).toHaveLength(1);
    expect(board.grid[0]).toEqual(expect.arrayContaining([43, 44]));
  });
});

describe("countByColumn", () => {
  it("counts merging cards in Ready to merge", () => {
    const counts = countByColumn(M.cardsOf("api"));
    expect(counts.ready).toBe(2);
    expect(Object.values(counts).reduce((a, b) => a + b, 0)).toBe(11);
  });
});

describe("column copy", () => {
  it("allows quick add in Backlog, Planning, and Working only", () => {
    expect(M.COLUMNS.filter(canQuickAdd)).toEqual(["backlog", "planning", "working"]);
  });

  it("says what starting the card does", () => {
    expect(addHint("backlog")).toBe("The card waits in Backlog until you start it.");
    expect(addHint("planning")).toBe(
      "The agent starts in plan first mode and posts a plan for you.",
    );
    expect(addHint("working")).toBe("The agent starts working on this card right away.");
    expect(addLabel("backlog")).toBe("Add card");
    expect(addLabel("working")).toBe("Add and start");
  });

  it("pluralizes the lane count", () => {
    expect(cardCountLabel(1)).toBe("1 card");
    expect(cardCountLabel(0)).toBe("0 cards");
    expect(cardCountLabel(12)).toBe("12 cards");
  });

  it("shows a drop hint while dragging, No cards on a plain board, and nothing in lanes", () => {
    expect(emptyText(true, "role")).toBe("Drop here");
    expect(emptyText(false, "none")).toBe("No cards");
    expect(emptyText(false, "role")).toBe("");
    expect(emptyText(false, undefined)).toBe("");
  });

  it("sizes columns: tall on a plain board, short in lanes", () => {
    expect(columnMinHeightClass("none", false)).toBe("min-h-110");
    expect(columnMinHeightClass("none", true)).toBe("min-h-60");
    expect(columnMinHeightClass("role", false)).toBe("min-h-18");
    expect(columnMinHeightClass(undefined, false)).toBe("min-h-18");
  });
});
