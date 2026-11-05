import { describe, expect, it, vi } from "vitest";
import { createUpdateStore, type UpdateInvoke, type UpdateSnapshot } from "./update-store";

const snapshot = (status: UpdateSnapshot["status"]): UpdateSnapshot => ({
  status,
  currentVersion: "1.0.0",
  version: "1.1.0",
  notes: "Improved cards",
  downloaded: 0,
});
function bridge(handler: (command: string) => unknown) {
  return vi.fn(async (command: string) => handler(command)) as unknown as UpdateInvoke;
}

describe("app update lifecycle", () => {
  it("does not attempt native installation in a browser", async () => {
    const store = createUpdateStore(null);
    await store.initialize();
    await store.install();
    expect(store.state.status).toBe("web");
  });
  it("checks on launch and exposes release notes without downloading", async () => {
    const invoke = bridge((command) =>
      snapshot(command === "update_status" ? "idle" : "available"),
    );
    const store = createUpdateStore(invoke);
    await store.initialize();
    expect(store.state.notes).toBe("Improved cards");
    expect(invoke).toHaveBeenCalledWith("check_update");
    expect(invoke).not.toHaveBeenCalledWith("install_update");
    await store.install();
    expect(invoke).toHaveBeenCalledWith("install_update");
  });
  it("preserves an installed update across reload without checking again", async () => {
    const invoke = bridge(() => snapshot("installed"));
    const store = createUpdateStore(invoke);
    await store.initialize();
    expect(invoke).toHaveBeenCalledTimes(1);
    await store.restart();
    expect(invoke).toHaveBeenCalledWith("restart_updated_app");
  });
  it("reports signature/download failures and allows checking again", async () => {
    const invoke = bridge((command) => {
      if (command === "install_update") throw new Error("Signature verification failed");
      return snapshot("available");
    });
    const store = createUpdateStore(invoke);
    await store.check();
    await store.install();
    expect(store.state.status).toBe("error");
    expect(store.state.error).toContain("Signature verification failed");
    await store.check();
    expect(store.state.status).toBe("available");
    expect(store.state.error).toBeNull();
  });
  it("prevents duplicate clicks while checking", async () => {
    let resolve!: (value: UpdateSnapshot) => void;
    const invoke = vi.fn(
      () =>
        new Promise<UpdateSnapshot>((done) => {
          resolve = done;
        }),
    ) as unknown as UpdateInvoke;
    const store = createUpdateStore(invoke);
    const first = store.check();
    await store.check();
    expect(invoke).toHaveBeenCalledTimes(1);
    resolve(snapshot("current"));
    await first;
    expect(store.state.status).toBe("current");
  });
});
