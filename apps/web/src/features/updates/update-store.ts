import { createStore } from "solid-js/store";

const STATUS_POLL_MS = 500;

export type UpdateStatus =
  | "web"
  | "idle"
  | "disabled"
  | "checking"
  | "current"
  | "available"
  | "downloading"
  | "installing"
  | "installed"
  | "restarting"
  | "error";
export interface UpdateSnapshot {
  status: UpdateStatus;
  currentVersion: string;
  version?: string | null;
  notes?: string | null;
  downloaded: number;
  total?: number | null;
  error?: string | null;
}
export type UpdateInvoke = <T>(command: string) => Promise<T>;
export const updateBusy = (status: UpdateStatus) =>
  ["checking", "downloading", "installing", "restarting"].includes(status);

/** One source of truth for navigation, prompts, and Settings. Native state survives page reloads. */
export function createUpdateStore(invoke: UpdateInvoke | null) {
  const [state, set] = createStore<UpdateSnapshot>({
    status: invoke ? "idle" : "web",
    currentVersion: "",
    downloaded: 0,
  });
  let pending = false;
  const fail = (error: unknown) =>
    set({ status: "error", error: error instanceof Error ? error.message : String(error) });
  async function refresh() {
    if (!invoke) return;
    try {
      set(await invoke<UpdateSnapshot>("update_status"));
    } catch (error) {
      fail(error);
    }
  }
  async function run(command: string, status: UpdateStatus) {
    if (!invoke || pending || updateBusy(state.status)) return;
    pending = true;
    set({ status, error: null });
    // Poll only during a long operation. The native mutex prevents duplicate downloads across reloads.
    const timer = setInterval(() => void refresh(), STATUS_POLL_MS);
    try {
      const result = await invoke<UpdateSnapshot | null>(command);
      if (result) set(result);
    } catch (error) {
      if (command === "restart_updated_app") set({ status: "installed", error: String(error) });
      else fail(error);
    } finally {
      clearInterval(timer);
      pending = false;
    }
  }
  return {
    state,
    refresh,
    async initialize() {
      await refresh();
      if (["idle", "current", "error"].includes(state.status))
        await run("check_update", "checking");
    },
    check: () => run("check_update", "checking"),
    install: () =>
      state.status === "available" ? run("install_update", "downloading") : Promise.resolve(),
    restart: () =>
      state.status === "installed" ? run("restart_updated_app", "restarting") : Promise.resolve(),
  };
}
