import { compareSortValues, sortDirection, toggleSort } from "./table-sort";

describe("sortDirection", () => {
  it("is none for other columns and ascending or descending for the sorted one", () => {
    expect(sortDirection({ k: "state", dir: 1 }, "role")).toBe("none");
    expect(sortDirection({ k: "state", dir: 1 }, "state")).toBe("ascending");
    expect(sortDirection({ k: "id", dir: -1 }, "id")).toBe("descending");
  });
});

describe("toggleSort", () => {
  it("flips the direction of the sorted column", () => {
    expect(toggleSort({ k: "id", dir: -1 }, "id")).toEqual({ k: "id", dir: 1 });
    expect(toggleSort({ k: "id", dir: 1 }, "id")).toEqual({ k: "id", dir: -1 });
  });

  it("starts another column ascending", () => {
    expect(toggleSort({ k: "id", dir: -1 }, "cost")).toEqual({ k: "cost", dir: 1 });
  });
});

describe("compareSortValues", () => {
  it("orders numbers by value", () => {
    expect(compareSortValues(1, 2)).toBe(-1);
    expect(compareSortValues(10, 9)).toBe(1);
    expect(compareSortValues(3, 3)).toBe(0);
  });

  it("orders text by code unit, so capitals come first", () => {
    expect(compareSortValues("Zed", "apple")).toBe(-1);
    expect(compareSortValues("b", "a")).toBe(1);
    expect(compareSortValues("", "")).toBe(0);
  });
});
