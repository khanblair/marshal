// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the person's sections are the daemon's).
import { daemon } from "~/testing/daemon-person-store";
import { cleanup, render, screen } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Card, M } from "~/mock";
import { MembersRow } from "./MembersRow";
import { createPanelState } from "./panel-state";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

afterEach(cleanup);

/** The fields of a card the row reads: who is on it, and whether it has started. */
const cardWith = (members: string[]) =>
  ({ members, state: "backlog", agent: "Claude Code", role: "Worker" }) as unknown as Card;

describe("the member picker on the daemon", () => {
  it("lists the daemon's users where it listed the prototype's people, and nobody else", () => {
    const panel = createPanelState();
    panel.set({ membersOpen: true });
    render(() => <MembersRow card={cardWith([])} panel={panel} />);
    const people = screen.getAllByRole("menuitemcheckbox").map((item) => item.textContent);
    // Each row is the avatar's initials and then the name.
    expect(people).toEqual(["AOAda Okafor"]);
    expect(screen.queryByText("Blair Akandwanaho")).toBeNull();
  });

  it("shows the members it can name as chips, and leaves out one the daemon does not know", () => {
    const owner = M.S.people[0]?.id ?? "";
    render(() => <MembersRow card={cardWith([owner, "blair"])} panel={createPanelState()} />);
    expect(screen.getByTitle("Ada Okafor")).toBeInTheDocument();
    expect(screen.queryByTitle("Blair Akandwanaho")).toBeNull();
  });

  it("names the signed-in person by the daemon's id, so what they write is theirs", () => {
    expect(M.meId()).toBe(daemon.me.profile.id);
    expect(M.person(M.meId())?.name).toBe("Ada Okafor");
  });
});
