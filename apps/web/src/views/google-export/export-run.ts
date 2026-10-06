import { createSignal } from "solid-js";
import { M } from "~/mock";
import { platform } from "~/platform";
import type { GoogleFileAnswer } from "~/sync/google-files-actions";
import { googleFileSpec } from "~/views/settings/google-files-spec";

/** One request to make a file in Google: what it is called, which connection it needs, and the call. */
export interface ExportJob {
  /** What is running and for what, such as `doc:api#41`. Two jobs with one key never run at once. */
  key: string;
  /** The connection the file needs: `gdocs`, `gsheets`, `gslides` or `gdrive`. */
  service: string;
  make: () => Promise<GoogleFileAnswer>;
}

const [running, setRunning] = createSignal<ReadonlySet<string>>(new Set());

/** True while a job with this key runs. Read it where a button is drawn, so the button follows. */
export const isExporting = (key: string): boolean => running().has(key);

function setBusy(key: string, busy: boolean): void {
  const next = new Set(running());
  if (busy) next.add(key);
  else next.delete(key);
  setRunning(next);
}

const serviceName = (service: string): string => googleFileSpec(service)?.name ?? service;

/**
 * The sentence for a connection that cannot be used yet, or null when it can. Read the store's own
 * list: the daemon is only asked once the connection reads connected.
 */
export function connectionProblem(service: string): string | null {
  const state = M.S.integrations.find((row) => row.id === service)?.st;
  if (state === "connected") return null;
  const verb = state === "error" ? "Reconnect" : "Connect";
  return `${verb} ${serviceName(service)} in Settings, under Integrations.`;
}

/** A toast action that opens a file in Google. It runs from the click on the toast, so no pop-up is blocked. */
function openAction(url: string): { label: string; run: () => void } | undefined {
  if (!/^https:\/\//i.test(url)) return undefined;
  return { label: "Open", run: () => void platform().openExternal(url) };
}

/**
 * Makes one file in Google and says how it went in a toast. It does nothing while the same job
 * already runs, and asks the daemon only when the connection reads connected. True when a file was made.
 */
export async function runExport(job: ExportJob): Promise<boolean> {
  if (isExporting(job.key)) return false;
  const problem = connectionProblem(job.service);
  if (problem) {
    M.toast(problem);
    return false;
  }
  setBusy(job.key, true);
  try {
    const answer = await job.make();
    if ("error" in answer) {
      M.toast(answer.error);
      return false;
    }
    M.toast(`Saved to ${serviceName(job.service)}`, openAction(answer.file.url));
    return true;
  } finally {
    setBusy(job.key, false);
  }
}
