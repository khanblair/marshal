// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the roles are the daemon's).
import { daemon, resetRoles } from "~/testing/daemon-roles-store";
import type { RoleList } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toExports, toExport, toRole } from "~/data/mappers/roles";
import { golden } from "~/data/testing/golden";
import { M } from "~/mock";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * Section S27: the Roles screen against the fake daemon. The daemon answers with the golden
 * `role-list`, so the list is Worker and Reviewer (Marshal's own, which are reset rather than
 * deleted) and Nightly janitor (a person's, which is deleted). Every write goes through the daemon
 * and the screen is redrawn from the whole list it answers, so the assertions below are on the
 * request and on what the screen shows after it comes back.
 */

const LIST = golden<RoleList>("role-list");
/** Worker as the daemon sends it, and as the screen holds it: the body is a save's, the row is a field's. */
const WORKER = LIST.roles[0] as RoleList["roles"][number];
const WORKER_ROLE = toRole(WORKER);
const ROWS = LIST.roles.map(toRole);
const NAME = "Name";
const DESCRIPTION = "Description";

beforeEach(() => {
  resetRoles();
  M.set({ settingsSection: "roles" });
});
afterEach(cleanup);

const names = (): string[] => M.S.roles.map((role) => role.name);
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
/** The role list itself. A `Select`'s own `<option>` elements are `option`s too, so it is scoped. */
const rows = (): HTMLElement[] =>
  within(screen.getByRole("listbox", { name: "Roles" })).getAllByRole("option");
/**
 * The names the list shows, in order. A row's first line is the name and nothing else: the Starter
 * badge and the model line are their own elements, so the name is the first span's own text.
 */
const rowNames = (): string[] =>
  rows().map((row) => row.querySelector("span")?.firstChild?.textContent?.trim() ?? "");
const editorName = (): string => screen.getByRole("heading", { level: 3 }).textContent ?? "";
const option = (name: string): HTMLElement =>
  screen.getByRole("option", { name: new RegExp(name) });
const field = (label: string): HTMLInputElement => screen.getByLabelText<HTMLInputElement>(label);
const click = (name: string | RegExp): void => {
  fireEvent.click(screen.getByRole("button", { name }));
};
const save = (): void => click("Save role");
const pick = (name: string): void => {
  fireEvent.click(option(name));
};
const type = (label: string, value: string): void => {
  fireEvent.input(field(label), { target: { value } });
};

describe("the Roles screen on the daemon", () => {
  it("shows the daemon's roles, and reset only on Marshal's own", () => {
    render(() => <SettingsView />);
    expect(rowNames()).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
    expect(editorName()).toBe("Worker");
    // Worker is Marshal's, so it is reset rather than deleted; nothing is unsaved, so Save is off.
    expect(screen.getByRole("button", { name: "Reset to starter" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete role" })).toBeNull();
    expect(screen.getByRole("button", { name: "Save role" })).toBeDisabled();
    // The person's own role is the other way round.
    pick("Nightly janitor");
    expect(screen.getByRole("button", { name: "Delete role" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reset to starter" })).toBeNull();
  });

  it("saves an edit with one PATCH, and redraws from the list the daemon answers", async () => {
    render(() => <SettingsView />);
    type(DESCRIPTION, "Does the coding on a card, and keeps commits small");
    save();
    await waitFor(() => expect(toasts()).toContain("Role saved"));
    expect(daemon.bodies("PATCH /v1/roles/Worker")).toEqual([
      { spec: { ...WORKER.spec, desc: "Does the coding on a card, and keeps commits small" } },
    ]);
    expect(field(DESCRIPTION)).toHaveValue("Does the coding on a card, and keeps commits small");
    expect(screen.getByRole("button", { name: "Save role" })).toBeDisabled();
  });

  it("renames through the daemon, and the editor follows the role to its new name", async () => {
    render(() => <SettingsView />);
    type(NAME, "Builder");
    save();
    await waitFor(() => expect(toasts()).toContain("Role saved"));
    expect(daemon.bodies("PATCH /v1/roles/Worker")).toEqual([
      { spec: WORKER.spec, name: "Builder" },
    ]);
    expect(names()).toEqual(["Builder", "Reviewer", "Nightly janitor"]);
    // The renamed role is still the one on screen: the editor does not fall back to the first role.
    expect(M.S.roleSel).toBe("Builder");
    expect(editorName()).toBe("Builder");
  });

  it("shows the daemon's sentence when a rename collides, and keeps the edit", async () => {
    render(() => <SettingsView />);
    type(NAME, "Reviewer");
    save();
    await waitFor(() =>
      expect(toasts()).toContain('A role called "Reviewer" already exists. Choose another name.'),
    );
    expect(names()).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
    // Nothing was lost: the field still holds what was typed, so it can be corrected.
    expect(field(NAME)).toHaveValue("Reviewer");
  });

  it("adds a new role from Worker, and picks it", async () => {
    render(() => <SettingsView />);
    click("New role");
    // The store is redrawn from the list the daemon answers before the pick lands, so both are waited
    // for: the role is picked in the same breath as the list it arrived in.
    await waitFor(() => {
      expect(names()).toContain("New role");
      expect(M.S.roleSel).toBe("New role");
    });
    expect(daemon.bodies("POST /v1/roles")).toEqual([{ name: "New role", spec: WORKER.spec }]);
    expect(editorName()).toBe("New role");
  });

  it("duplicates the role as the form shows it, unsaved edits and all", async () => {
    render(() => <SettingsView />);
    type(DESCRIPTION, "Mine");
    click("Duplicate");
    await waitFor(() => {
      expect(names()).toContain("Worker copy");
      expect(M.S.roleSel).toBe("Worker copy");
    });
    expect(daemon.bodies("POST /v1/roles")).toEqual([
      { name: "Worker copy", spec: { ...WORKER.spec, desc: "Mine" } },
    ]);
    expect(field(DESCRIPTION)).toHaveValue("Mine");
  });

  it("resets a starter role for every project, and leaves the role's own body alone", async () => {
    render(() => <SettingsView />);
    click("Reset to starter");
    expect(M.S.dialog?.title).toBe("Reset to starter");
    expect(M.S.dialog?.message).toBe(
      "This replaces your edits to Worker with the starter template.",
    );
    M.S.dialog?.run();
    await waitFor(() => expect(toasts()).toContain("Role reset"));
    expect(
      daemon.routes().filter((route) => route.startsWith("POST /v1/roles/Worker/reset")),
    ).toEqual([
      "POST /v1/roles/Worker/reset?project=api",
      "POST /v1/roles/Worker/reset?project=web",
      "POST /v1/roles/Worker/reset?project=mobile",
    ]);
    // The reset clears each project's own version; the role itself is exactly as it was.
    expect(names()).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
    expect(field(DESCRIPTION)).toHaveValue("Does the coding on a card");
  });

  it("deletes a role a person made, and falls back to Worker", async () => {
    render(() => <SettingsView />);
    pick("Nightly janitor");
    click("Delete role");
    expect(M.S.dialog?.message).toBe(
      "This deletes the Nightly janitor role. Cards using it switch to Worker.",
    );
    M.S.dialog?.run();
    await waitFor(() => expect(toasts()).toContain("Role deleted"));
    expect(daemon.routes()).toContain("DELETE /v1/roles/Nightly%20janitor");
    expect(names()).toEqual(["Worker", "Reviewer"]);
    expect(M.S.roleSel).toBe("Worker");
    expect(editorName()).toBe("Worker");
  });
});

describe("exporting and importing roles", () => {
  // The `Field` is a wrapping label, so the control's name carries the error text too once one is
  // shown; the label itself is always the first thing in it.
  const box = (): HTMLTextAreaElement => screen.getByRole("textbox", { name: /^Roles/ });
  /** The dialog's Import, not the section's: the section keeps its own Import button behind it. */
  const importButton = (): HTMLElement =>
    within(screen.getByRole("dialog")).getByRole("button", { name: "Import" });

  it("exports every role as the document an import reads, without the daemon's own fields", () => {
    render(() => <SettingsView />);
    click("Export");
    expect(screen.getByText("Export roles")).toBeInTheDocument();
    expect(JSON.parse(box().value)).toEqual(toExports(ROWS));
    // The id and the two flags are the daemon's to set, so they are not in the document.
    expect(box().value).not.toContain("01JD7Q4M2X8K9V0P5T3RB6NHAE");
    expect(box().value).not.toContain("starter");
    expect(box().value).not.toContain("overridden");
  });

  it("imports a pasted role and says how many landed", async () => {
    render(() => <SettingsView />);
    click("Import");
    const pasted = { ...toExport(WORKER_ROLE), name: "Sweeper" };
    fireEvent.input(box(), { target: { value: JSON.stringify(pasted) } });
    fireEvent.click(importButton());
    await waitFor(() => expect(toasts()).toContain("Imported 1 role"));
    expect(daemon.bodies("POST /v1/roles")).toEqual([pasted]);
    expect(names()).toContain("Sweeper");
    // The dialog closed, so the list is the thing being looked at again.
    expect(screen.queryByText("Import roles")).toBeNull();
  });

  it("imports several at once and counts them", async () => {
    render(() => <SettingsView />);
    click("Import");
    const pasted = [
      { ...toExport(WORKER_ROLE), name: "One" },
      { ...toExport(WORKER_ROLE), name: "Two" },
    ];
    fireEvent.input(box(), { target: { value: JSON.stringify(pasted) } });
    fireEvent.click(importButton());
    await waitFor(() => expect(toasts()).toContain("Imported 2 roles"));
    expect(daemon.bodies("POST /v1/roles")).toEqual(pasted);
  });

  it("says what is wrong with the pasted text itself, without asking the daemon", () => {
    render(() => <SettingsView />);
    click("Import");
    fireEvent.click(importButton());
    expect(screen.getByText("Paste what Export gave you first.")).toBeInTheDocument();
    fireEvent.input(box(), { target: { value: "not json" } });
    fireEvent.click(importButton());
    expect(
      screen.getByText("That is not JSON. Paste what Export gave you, or a role's own export."),
    ).toBeInTheDocument();
    expect(daemon.routes()).not.toContain("POST /v1/roles");
  });

  it("keeps the dialog open and shows the daemon's sentence when a name is taken", async () => {
    render(() => <SettingsView />);
    click("Import");
    fireEvent.input(box(), { target: { value: JSON.stringify(toExport(WORKER_ROLE)) } });
    fireEvent.click(importButton());
    await waitFor(() =>
      expect(toasts()).toContain('A role called "Worker" already exists. Choose another name.'),
    );
    expect(names()).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
    // Still open, with the paste in it, so the name can be changed rather than the export retyped.
    expect(screen.getByText("Import roles")).toBeInTheDocument();
    expect(box()).toHaveValue(JSON.stringify(toExport(WORKER_ROLE)));
  });
});
