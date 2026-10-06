import { describe, expect, it } from "vitest";
import { boardSlides, MAX_BULLETS, MAX_SLIDES, type SlideColumn } from "./board-slides";

const titles = (count: number, prefix = "Card"): string[] =>
  Array.from({ length: count }, (_, index) => `${prefix} ${index + 1}`);

const column = (label: string, count: number): SlideColumn => ({ label, titles: titles(count) });

describe("boardSlides", () => {
  it("opens with the project and its counts, then a slide for each column", () => {
    const deck = boardSlides({
      project: "api-gateway",
      columns: [column("Backlog", 2), column("Working", 1), column("Done", 0)],
    });
    expect(deck.title).toBe("api-gateway board");
    expect(deck.slides).toHaveLength(4);
    expect(deck.slides[0]).toEqual({
      title: "api-gateway",
      bullets: ["3 cards", "Backlog: 2", "Working: 1"],
    });
    expect(deck.slides[1]).toEqual({ title: "Backlog (2)", bullets: ["Card 1", "Card 2"] });
    expect(deck.slides[2]).toEqual({ title: "Working (1)", bullets: ["Card 1"] });
  });

  it("says no cards for an empty column, and counts one card in the singular", () => {
    const deck = boardSlides({ project: "p", columns: [column("Working", 1), column("Done", 0)] });
    expect(deck.slides[0]?.bullets[0]).toBe("1 card");
    expect(deck.slides[2]).toEqual({ title: "Done (0)", bullets: ["No cards."] });
  });

  it("shows every card of a column that has exactly the most a slide holds", () => {
    const deck = boardSlides({ project: "p", columns: [column("Backlog", MAX_BULLETS)] });
    expect(deck.slides[1]?.bullets).toHaveLength(MAX_BULLETS);
    expect(deck.slides[1]?.bullets.at(-1)).toBe(`Card ${MAX_BULLETS}`);
  });

  it("cuts a longer column to the most a slide holds, the last line saying how many are left out", () => {
    const deck = boardSlides({ project: "p", columns: [column("Backlog", 60)] });
    const bullets = deck.slides[1]?.bullets ?? [];
    expect(bullets).toHaveLength(MAX_BULLETS);
    expect(bullets[MAX_BULLETS - 2]).toBe("Card 49");
    expect(bullets.at(-1)).toBe("and 11 more");
    expect(deck.slides[1]?.title).toBe("Backlog (60)");
  });

  it("keeps a presentation to the most slides one holds", () => {
    const columns = Array.from({ length: 150 }, (_, index) => column(`Column ${index}`, 1));
    const deck = boardSlides({ project: "p", columns });
    expect(deck.slides).toHaveLength(MAX_SLIDES);
    expect(deck.slides[0]?.bullets).toHaveLength(MAX_BULLETS);
    expect(deck.slides[0]?.bullets.at(-1)).toMatch(/^and \d+ more$/);
  });

  it("keeps markup, formulas and line breaks in a title as plain text on one line", () => {
    const deck = boardSlides({
      project: "<b>p</b> & co",
      columns: [
        { label: "Backlog", titles: ['=HYPERLINK("x")', "<script>a</script>", "One\nTwo"] },
      ],
    });
    expect(deck.title).toBe("<b>p</b> & co board");
    expect(deck.slides[1]?.bullets).toEqual(['=HYPERLINK("x")', "<script>a</script>", "One Two"]);
  });
});
