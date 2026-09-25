import { describe, expect, it } from "vitest";
import { readKey, writeKey } from "./storage";
import { brokenStorage, memoryStorage } from "./testing/memory-storage";

describe("readKey and writeKey", () => {
  it("reads what was written and removes on null", () => {
    const storage = memoryStorage();
    writeKey(storage, "k", "v");
    expect(readKey(storage, "k")).toBe("v");
    writeKey(storage, "k", null);
    expect(readKey(storage, "k")).toBeNull();
  });

  it("does nothing when there is no storage", () => {
    expect(readKey(null, "k")).toBeNull();
    expect(() => writeKey(null, "k", "v")).not.toThrow();
  });

  it("does not throw when the storage does", () => {
    const storage = brokenStorage();
    expect(readKey(storage, "k")).toBeNull();
    expect(() => writeKey(storage, "k", "v")).not.toThrow();
    expect(() => writeKey(storage, "k", null)).not.toThrow();
  });
});
