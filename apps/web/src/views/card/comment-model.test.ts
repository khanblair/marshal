import { beforeEach, describe, expect, it, vi } from "vitest";
import { attachmentsFrom, commentViews, initialsOf, linkAttachment } from "./comment-model";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

describe("commentViews", () => {
  beforeEach(() => resetStore());

  it("lists comments newest first and names the authors", () => {
    const views = commentViews(cardOf("api#41"));
    expect(views).toHaveLength(cardOf("api#41").comments.length);
    expect(views.map((v) => v.name)).toContain("Claude Code agent");
    expect(views.map((v) => v.name)).toContain("Blair Akandwanaho");
    expect(views.find((v) => v.isAgent)?.initials).toBe("");
  });

  it("splits images from files and links, and only lets you delete your own", () => {
    const views = commentViews(cardOf("api#41"));
    const withImage = views.find((v) => v.images.length > 0);
    expect(withImage?.images[0]).toEqual({ name: "logout-after-refresh.png", src: "" });
    expect(views.some((v) => v.mine)).toBe(true);
    const plan = commentViews(cardOf("api#43")).find((v) => v.files.length > 0);
    expect(plan?.files[0]).toMatchObject({
      name: "plan-tiers-2026.xlsx",
      icon: "file-text",
      meta: "48 KB",
      href: "#",
      placeholder: true,
      isLink: false,
    });
  });

  it("marks a link attachment with the link icon and its address", () => {
    const link = commentViews(cardOf("web#118"))
      .flatMap((v) => v.files)
      .find((f) => f.isLink);
    expect(link).toMatchObject({
      icon: "link",
      href: "https://figma.com/file/settings-dark",
      placeholder: false,
    });
  });

  it("shows Agent read this only for read comments of people on a started card", () => {
    expect(commentViews(cardOf("api#41")).some((v) => v.readShow)).toBe(true);
    const backlog = cardOf("api#45");
    backlog.comments.push({
      id: "c1",
      author: "ada",
      text: "Hi",
      ts: Date.now(),
      att: [],
      read: true,
    });
    expect(commentViews(backlog).every((v) => !v.readShow)).toBe(true);
  });
});

describe("attachments", () => {
  it("makes initials of at most two letters", () => {
    expect(initialsOf("Blair Akandwanaho")).toBe("BA");
    expect(initialsOf("Ada Lovelace King")).toBe("AL");
  });

  it("turns a typed link into an https attachment", () => {
    expect(linkAttachment("example.com/spec")).toEqual({
      kind: "link",
      name: "example.com/spec",
      url: "https://example.com/spec",
    });
    expect(linkAttachment("http://a.dev/x").url).toBe("http://a.dev/x");
  });

  it("describes picked files by kind and size", () => {
    vi.stubGlobal("URL", { createObjectURL: () => "blob:x" });
    const image = new File(["x".repeat(2_500_000)], "shot.png", { type: "image/png" });
    const note = new File(["hello"], "notes.txt", { type: "text/plain" });
    const list = {
      0: image,
      1: note,
      length: 2,
      item: () => null,
      [Symbol.iterator]: Array.prototype[Symbol.iterator],
    } as unknown as FileList;
    expect(attachmentsFrom(list)).toEqual([
      { kind: "image", name: "shot.png", size: "2.5 MB", src: "blob:x" },
      { kind: "file", name: "notes.txt", size: "1 KB", src: "blob:x" },
    ]);
    expect(attachmentsFrom(null)).toEqual([]);
    vi.unstubAllGlobals();
  });
});
