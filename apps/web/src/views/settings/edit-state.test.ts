import { createRoot } from "solid-js";
import { describe, expect, it } from "vitest";
import { createEditState } from "./edit-state";

const inRoot = <T>(fn: () => T): T =>
  createRoot((dispose) => {
    const value = fn();
    dispose();
    return value;
  });

describe("createEditState", () => {
  it("opens a row, clearing the last error", () => {
    inRoot(() => {
      const edit = createEditState();
      edit.open("openai");
      edit.fail("Too short");
      expect(edit.errorFor("openai")).toBe("Too short");
      edit.open("openai");
      expect(edit.id()).toBe("openai");
      expect(edit.errorFor("openai")).toBeNull();
    });
  });

  it("shows an error only for the open row", () => {
    inRoot(() => {
      const edit = createEditState();
      edit.open("a");
      edit.fail("Bad");
      expect(edit.errorFor("b")).toBeNull();
    });
  });

  it("toggles a row open and closed", () => {
    inRoot(() => {
      const edit = createEditState();
      edit.toggle("s1");
      expect(edit.id()).toBe("s1");
      edit.toggle("s2");
      expect(edit.id()).toBe("s2");
      edit.toggle("s2");
      expect(edit.id()).toBeNull();
    });
  });

  it("moves to another row and keeps the error, and closes with no error left", () => {
    inRoot(() => {
      const edit = createEditState();
      edit.open("s1");
      edit.fail("Can't read this time");
      edit.moveTo("s2");
      expect(edit.errorFor("s2")).toBe("Can't read this time");
      edit.close();
      expect(edit.id()).toBeNull();
      expect(edit.errorFor("s2")).toBeNull();
    });
  });
});
