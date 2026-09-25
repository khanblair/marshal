import type { KeyValueStore } from "../storage";

export interface MemoryStorage extends KeyValueStore {
  values: Map<string, string>;
  writes: string[];
}

/** A `localStorage` that lives in memory and records what was written, for tests. */
export function memoryStorage(initial: Record<string, string> = {}): MemoryStorage {
  const values = new Map(Object.entries(initial));
  const writes: string[] = [];
  return {
    values,
    writes,
    getItem: (key) => values.get(key) ?? null,
    setItem(key, value) {
      writes.push(key);
      values.set(key, value);
    },
    removeItem(key) {
      writes.push(key);
      values.delete(key);
    },
  };
}

/** A storage that throws on every use, like `localStorage` with site data blocked. */
export function brokenStorage(): KeyValueStore {
  const fail = () => {
    throw new DOMException("Storage is blocked.", "SecurityError");
  };
  return { getItem: fail, setItem: fail, removeItem: fail };
}
