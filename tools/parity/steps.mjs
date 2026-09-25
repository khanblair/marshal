/** Shared scenario steps. They only use `window.M`, which both apps expose. */

export const go = (page, ...args) => page.evaluate((a) => window.M.go(...a), args);
export const openCard = (page, id, tab) => page.evaluate(([i, t]) => window.M.openCard(i, t), [id, tab]);
export const setView = (page, view) => page.evaluate((v) => window.M.setView(v), view);
export const set = (page, patch) => page.evaluate((p) => window.M.set(p), patch);

/** Opens a project view: `project("api", "board")`. */
export const project = (pid, view) => async (page) => go(page, "project", pid, view);
