/** The subset of `localStorage` the store uses. */
export interface KeyValueStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

/** Present once the user finished onboarding. */
export const ONBOARDED_KEY = "marshal-proto-onboarded";

/* Storage can be missing or throw (private mode, blocked site data), so every access is guarded. */

export function readKey(storage: KeyValueStore | null, key: string): string | null {
  try {
    return storage ? storage.getItem(key) : null;
  } catch {
    return null;
  }
}

export function writeKey(storage: KeyValueStore | null, key: string, value: string | null): void {
  try {
    if (!storage) return;
    if (value === null) storage.removeItem(key);
    else storage.setItem(key, value);
  } catch {
    /* Storage is optional: the app keeps working without it. */
  }
}
