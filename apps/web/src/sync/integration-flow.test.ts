import { describe, expect, it } from "vitest";
import {
  openCardWorktree,
  pauseMerging,
  resumeMerging,
  retryMerge,
  undoMerge,
} from "./integration-flow";

/*
 * No store in this file follows a daemon, so there is nothing for the actions to ask. They answer
 * false and send nothing, which is what the mock's own store gets from them.
 */
describe("the merge flow's actions with no daemon", () => {
  it("do nothing and say so", async () => {
    expect(await retryMerge("web#12")).toBe(false);
    expect(await undoMerge("web#12")).toBe(false);
    expect(await openCardWorktree("web#12", "finder")).toBe(false);
    expect(await pauseMerging("web")).toBe(false);
    expect(await resumeMerging("web")).toBe(false);
  });
});
