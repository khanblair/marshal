/** Deep copy through JSON, as the design does for every draft (functions and undefined drop out). */
export function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

/** Equal when the JSON matches, key order included. Drafts are clones, so the order always agrees. */
export function sameJson(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
