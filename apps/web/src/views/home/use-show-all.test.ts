import { createRoot, createSignal } from "solid-js";
import { describe, expect, it } from "vitest";
import { COLLAPSED_ROW_COUNT, createShowAll } from "./use-show-all";

const numbers = (count: number) => Array.from({ length: count }, (_, i) => i);

describe("createShowAll", () => {
  it("shows the first five rows until toggled, then all of them", () => {
    createRoot((dispose) => {
      const rows = createShowAll(() => numbers(8));
      expect(COLLAPSED_ROW_COUNT).toBe(5);
      expect(rows.expanded()).toBe(false);
      expect(rows.visible()).toEqual([0, 1, 2, 3, 4]);
      rows.toggle();
      expect(rows.expanded()).toBe(true);
      expect(rows.visible()).toHaveLength(8);
      rows.toggle();
      expect(rows.visible()).toHaveLength(5);
      dispose();
    });
  });

  it("follows the list as it changes", () => {
    createRoot((dispose) => {
      const [list, setList] = createSignal(numbers(3));
      const rows = createShowAll(list);
      expect(rows.visible()).toEqual([0, 1, 2]);
      setList(numbers(9));
      expect(rows.visible()).toHaveLength(5);
      rows.toggle();
      setList(numbers(10));
      expect(rows.visible()).toHaveLength(10);
      dispose();
    });
  });
});
