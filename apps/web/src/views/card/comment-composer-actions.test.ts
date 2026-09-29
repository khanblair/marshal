import { createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as platformModule from "~/platform";
import { createComposerActions } from "./comment-composer-actions";

afterEach(() => vi.restoreAllMocks());

function make(pending: unknown[] = []) {
  const state = { pending, cDraft: "", linkOpen: false };
  const panel = {
    get state() {
      return state;
    },
    set: vi.fn((patch: Partial<typeof state>) => Object.assign(state, patch)),
  };
  const actions = createRoot(() =>
    createComposerActions({ cardId: "api#1", panel } as never, () => undefined),
  );
  return { actions, state, panel };
}

describe("Take photo", () => {
  it("adds what the camera took as an image attachment", async () => {
    const photo = new File(["x"], "photo.jpg", { type: "image/jpeg" });
    const pickFiles = vi.fn().mockResolvedValue([photo]);
    vi.spyOn(platformModule, "platform").mockReturnValue({
      ...platformModule.createPlatform("web"),
      kind: "mobile",
      pickFiles,
    });
    URL.createObjectURL = vi.fn(() => "blob:test/1");
    const { actions, state } = make();
    actions.takePhoto();
    await vi.waitFor(() => expect(state.pending).toHaveLength(1));
    expect(pickFiles).toHaveBeenCalledWith({ accept: "image/*", camera: true });
    expect(state.pending[0]).toMatchObject({ kind: "image", name: "photo.jpg" });
  });

  it("adds nothing when the camera is closed without a photo", async () => {
    vi.spyOn(platformModule, "platform").mockReturnValue({
      ...platformModule.createPlatform("web"),
      pickFiles: vi.fn().mockResolvedValue([]),
    });
    const { actions, panel } = make();
    actions.takePhoto();
    await Promise.resolve();
    await Promise.resolve();
    expect(panel.set).not.toHaveBeenCalled();
  });
});
