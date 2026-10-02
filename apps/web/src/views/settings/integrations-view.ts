import { type KeyValueStore, readKey, writeKey } from "~/data/storage";

/** How the Integrations screen lays its connections out. */
export type IntegrationsView = "list" | "cards";

export const INTEGRATIONS_VIEW_KEY = "marshal.integrations.view";

function browserStore(): KeyValueStore | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

/** The layout this browser last chose; a list until it chooses another. */
export function readIntegrationsView(): IntegrationsView {
  return readKey(browserStore(), INTEGRATIONS_VIEW_KEY) === "cards" ? "cards" : "list";
}

export function writeIntegrationsView(view: IntegrationsView): void {
  writeKey(browserStore(), INTEGRATIONS_VIEW_KEY, view);
}

export const INTEGRATIONS_EXPANDED_KEY = "marshal.integrations.expanded";

/** The ids of the connections this browser unfolded; none until it unfolds one, so all start folded. */
export function readExpandedIntegrations(): string[] {
  try {
    const parsed: unknown = JSON.parse(readKey(browserStore(), INTEGRATIONS_EXPANDED_KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === "string") : [];
  } catch {
    return [];
  }
}

export function writeExpandedIntegrations(ids: readonly string[]): void {
  writeKey(browserStore(), INTEGRATIONS_EXPANDED_KEY, JSON.stringify(ids));
}
