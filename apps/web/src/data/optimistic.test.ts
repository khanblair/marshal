import { describe, expect, it, vi } from "vitest";
import { clientError, wireError } from "./api-error";
import { ChangeInFlightError, createOptimistic } from "./optimistic";

function setup() {
  const order: string[] = [];
  const toast = vi.fn((message: string) => {
    order.push(`toast:${message}`);
  });
  const optimistic = createOptimistic(toast);
  const options = (request: () => Promise<unknown>) => ({
    apply: () => order.push("apply"),
    rollback: () => order.push("rollback"),
    request: async () => {
      order.push("request");
      return request();
    },
  });
  return { order, toast, optimistic, options };
}

describe("optimistic", () => {
  it("applies first, then asks, and resolves with the result", async () => {
    const t = setup();
    const result = await t.optimistic({ ...t.options(async () => "saved") });
    expect(result).toBe("saved");
    expect(t.order).toEqual(["apply", "request"]);
    expect(t.toast).not.toHaveBeenCalled();
  });

  it("applies before the request is even started", async () => {
    const t = setup();
    let release: (value: string) => void = () => undefined;
    const pending = t.optimistic({
      ...t.options(() => new Promise<string>((resolve) => (release = resolve))),
    });
    expect(t.order).toEqual(["apply", "request"]);
    release("ok");
    await pending;
  });

  it("rolls back exactly once when the request fails, then shows one plain message, then rethrows", async () => {
    const t = setup();
    const failure = wireError(409, { code: "conflict", message: "That name is already in use." });
    await expect(t.optimistic({ ...t.options(async () => Promise.reject(failure)) })).rejects.toBe(
      failure,
    );
    expect(t.order).toEqual(["apply", "request", "rollback", "toast:That name is already in use."]);
    expect(t.toast).toHaveBeenCalledTimes(1);
  });

  it("uses the words of describeError when it gives some", async () => {
    const t = setup();
    const failure = clientError("unreachable");
    await expect(
      t.optimistic({
        ...t.options(async () => Promise.reject(failure)),
        describeError: () => "The card did not move.",
      }),
    ).rejects.toBe(failure);
    expect(t.toast).toHaveBeenCalledWith("The card did not move.");
  });

  it("falls back to the message of the error when describeError has nothing to say", async () => {
    const t = setup();
    await expect(
      t.optimistic({
        ...t.options(async () => Promise.reject(clientError("timeout"))),
        describeError: () => undefined,
      }),
    ).rejects.toThrow();
    expect(t.toast).toHaveBeenCalledWith(clientError("timeout").message);
  });

  it("never shows a stack or a code for an error it does not know", async () => {
    const t = setup();
    await expect(
      t.optimistic({
        ...t.options(async () => Promise.reject(new TypeError("x is not a function"))),
      }),
    ).rejects.toThrow(TypeError);
    const [message] = t.toast.mock.calls[0] ?? [""];
    expect(message).toBe("Marshal could not save that change. Try again.");
    expect(message).not.toContain("TypeError");
  });

  it("rolls back when apply itself fails, and does not ask the daemon", async () => {
    const t = setup();
    const request = vi.fn(async () => "never");
    await expect(
      t.optimistic({
        apply: () => {
          throw new RangeError("bad state");
        },
        request,
        rollback: () => t.order.push("rollback"),
      }),
    ).rejects.toThrow(RangeError);
    expect(request).not.toHaveBeenCalled();
    expect(t.order).toEqual(["rollback", "toast:Marshal could not save that change. Try again."]);
  });

  it("still shows the message when rollback fails", async () => {
    const t = setup();
    await expect(
      t.optimistic({
        apply: () => undefined,
        request: async () => Promise.reject(clientError("unreachable")),
        rollback: () => {
          throw new Error("rollback bug");
        },
      }),
    ).rejects.toThrow("rollback bug");
    expect(t.toast).toHaveBeenCalledTimes(1);
  });

  it("refuses a second call with the same key while the first is running, and allows it after", async () => {
    const t = setup();
    let release: (value: string) => void = () => undefined;
    const first = t.optimistic({
      key: "card-1",
      ...t.options(() => new Promise<string>((resolve) => (release = resolve))),
    });
    await expect(
      t.optimistic({ key: "card-1", ...t.options(async () => "second") }),
    ).rejects.toThrow("That change is still being saved. Wait a moment and try again.");
    // A caller can tell it apart from a failure: it is the one error that is not shown by itself.
    await expect(
      t.optimistic({ key: "card-1", ...t.options(async () => "third") }),
    ).rejects.toBeInstanceOf(ChangeInFlightError);
    // The refused call changed nothing on the screen.
    expect(t.order).toEqual(["apply", "request"]);
    // A different key is a different change.
    await expect(t.optimistic({ key: "card-2", ...t.options(async () => "other") })).resolves.toBe(
      "other",
    );
    release("first");
    await first;
    await expect(t.optimistic({ key: "card-1", ...t.options(async () => "again") })).resolves.toBe(
      "again",
    );
  });

  it("frees the key after a failure too", async () => {
    const t = setup();
    await expect(
      t.optimistic({ key: "k", ...t.options(async () => Promise.reject(clientError("timeout"))) }),
    ).rejects.toThrow();
    await expect(t.optimistic({ key: "k", ...t.options(async () => "ok") })).resolves.toBe("ok");
  });

  it("keeps the keys of one instance apart from another", async () => {
    const a = setup();
    const b = setup();
    let release: () => void = () => undefined;
    const first = a.optimistic({
      key: "k",
      ...a.options(() => new Promise<void>((resolve) => (release = resolve))),
    });
    await expect(b.optimistic({ key: "k", ...b.options(async () => "fine") })).resolves.toBe(
      "fine",
    );
    release();
    await first;
  });
});
