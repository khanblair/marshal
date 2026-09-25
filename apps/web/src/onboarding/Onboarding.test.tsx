import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { GOLDEN_CATALOG, wireAgent, wireCatalog } from "~/testing/agents";
import { useCatalog } from "~/testing/test-store";
import { Onboarding } from "./Onboarding";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const PHONE_WIDTH_PX = 390;
const DESKTOP_WIDTH_PX = 1440;
/** Longer than the eight characters below which a key is ignored. */
const LONG_KEY = "sk-ant-api03-abcdefgh1234";

const pristine = JSON.stringify({
  profile: M.S.profile,
  providers: M.S.providers,
  integrations: M.S.integrations,
  projects: M.S.projects,
  cards: M.S.cards,
});

beforeEach(() => {
  vi.useFakeTimers();
  M.set({
    ...JSON.parse(pristine),
    onboarding: true,
    obStep: 0,
    tour: null,
    theme: "system",
    toasts: [],
    vw: DESKTOP_WIDTH_PX,
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

const click = (name: string | RegExp, role = "button"): void => {
  fireEvent.click(screen.getByRole(role, { name }));
};
const type = (input: HTMLElement, value: string): void => {
  fireEvent.input(input, { target: { value } });
};
const nameInput = (): HTMLInputElement =>
  screen
    .getByRole("dialog")
    .querySelector<HTMLInputElement>('input[autocomplete="name"]') as HTMLInputElement;
const goTo = (step: number): void => {
  M.set({ obStep: step });
};

describe("Onboarding welcome screen", () => {
  it("is a modal dialog named by its title, on step 1 of 5", () => {
    render(() => <Onboarding />);
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAccessibleName("Welcome to Marshal");
    expect(screen.getByText("Step 1 of 5")).toHaveAttribute("aria-live", "polite");
  });

  it("shows the demo board with its four columns", () => {
    render(() => <Onboarding />);
    const board = screen.getByRole("img", {
      name: "A board with cards moving from planning to done",
    });
    for (const label of ["Planning", "Working", "Needs you", "Done"]) {
      expect(board).toHaveTextContent(label);
    }
  });

  it("has Skip and Continue but no Back", () => {
    render(() => <Onboarding />);
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
    expect(screen.getByRole("button", { name: "Skip" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument();
  });

  it("fills the fifth of the step indicator up to the current step", () => {
    const { container } = render(() => <Onboarding />);
    const dots = container.querySelectorAll('[aria-hidden="true"] > span');
    expect([...dots].map((dot) => dot.classList.contains("bg-ink"))).toEqual([
      true,
      false,
      false,
      false,
      false,
    ]);
  });
});

describe("Onboarding focus", () => {
  it("moves focus to Continue shortly after opening", () => {
    render(() => <Onboarding />);
    expect(screen.getByRole("button", { name: "Continue" })).not.toHaveFocus();
    vi.advanceTimersByTime(60);
    expect(screen.getByRole("button", { name: "Continue" })).toHaveFocus();
  });

  it("gives Continue focus again after each step", () => {
    render(() => <Onboarding />);
    click("Continue");
    screen.getByRole("button", { name: "Skip" }).focus();
    vi.advanceTimersByTime(30);
    expect(screen.getByRole("button", { name: "Continue" })).toHaveFocus();
  });
});

describe("Onboarding steps", () => {
  it("walks through the five screens with Continue and back with Back", () => {
    render(() => <Onboarding />);
    const titles = [
      "Set up your profile",
      "Connect your agents",
      "Add your first project",
      "Stay in control from anywhere",
    ];
    goTo(1);
    type(nameInput(), "Ada");
    for (const [index, title] of titles.slice(1).entries()) {
      click("Continue");
      expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(title);
      expect(screen.getByText(`Step ${index + 3} of 5`)).toBeInTheDocument();
    }
    expect(screen.getByRole("button", { name: "Open Marshal" })).toBeInTheDocument();
    click("Back");
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Add your first project");
    expect(M.S.obStep).toBe(3);
  });

  it("shows the screen for whatever step the store says", () => {
    render(() => <Onboarding />);
    goTo(2);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Connect your agents");
    goTo(4);
    expect(screen.getByText("Pair a phone over Tailscale")).toBeInTheDocument();
    goTo(0);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Welcome to Marshal");
  });

  it("Skip goes on without saving anything", () => {
    render(() => <Onboarding />);
    goTo(1);
    type(nameInput(), "Grace Hopper");
    const before = M.S.profile.name;
    click("Skip");
    expect(M.S.obStep).toBe(2);
    expect(M.S.profile.name).toBe(before);
  });
});

describe("Onboarding profile screen", () => {
  beforeEach(() => goTo(1));

  it("asks for a name when Continue is pressed with none, and stays put", () => {
    render(() => <Onboarding />);
    expect(screen.queryByText("Enter a name to continue.")).toBeNull();
    click("Continue");
    expect(screen.getByText("Enter a name to continue.")).toBeInTheDocument();
    expect(M.S.obStep).toBe(1);
  });

  it("drops the error as soon as a name is typed, and Continue then saves the profile", () => {
    render(() => <Onboarding />);
    click("Continue");
    type(nameInput(), "  Grace Hopper ");
    expect(screen.queryByText("Enter a name to continue.")).toBeNull();
    type(screen.getByPlaceholderText("Optional"), "grace@navy.mil");
    click("Continue");
    expect(M.S.profile).toMatchObject({ name: "Grace Hopper", email: "grace@navy.mil" });
    expect(M.S.obStep).toBe(2);
  });

  it("keeps the error after Back and Skip, as the design does, until Continue works", () => {
    render(() => <Onboarding />);
    click("Continue");
    click("Back");
    click("Skip");
    expect(screen.getByText("Enter a name to continue.")).toBeInTheDocument();
  });

  it("shows initials for the name, and a question mark without one", () => {
    render(() => <Onboarding />);
    expect(screen.getByLabelText("Avatar preview")).toHaveTextContent("?");
    type(nameInput(), "ada okafor");
    expect(screen.getByLabelText("Avatar preview")).toHaveTextContent("AO");
  });

  it("Upload image toasts and turns into Change image", () => {
    render(() => <Onboarding />);
    click("Upload image");
    expect(M.S.toasts.map((toast) => toast.msg)).toContain("Choose an image to use as your avatar");
    expect(screen.getByRole("button", { name: "Change image" })).toBeInTheDocument();
  });

  it("saves the chosen time zone", () => {
    render(() => <Onboarding />);
    type(nameInput(), "Ada");
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "Asia/Singapore" } });
    click("Continue");
    expect(M.S.profile.tz).toBe("Asia/Singapore");
  });

  it("switches the theme at once", () => {
    render(() => <Onboarding />);
    expect(screen.getByRole("radiogroup", { name: "Theme" })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "System" })).toHaveAttribute("aria-checked", "true");
    click("Dark", "radio");
    expect(M.S.theme).toBe("dark");
    expect(screen.getByRole("radio", { name: "Dark" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "System" })).toHaveAttribute("aria-checked", "false");
  });
});

describe("Onboarding agents screen", () => {
  beforeEach(() => goTo(2));

  it("lists the three agents found, with versions", () => {
    render(() => <Onboarding />);
    const rows = screen.getAllByRole("listitem");
    expect(rows.map((row) => row.textContent)).toEqual([
      "Claude Code2.0.14Found",
      "Codex0.42.0Found",
      "Gemini CLI0.8.1Found",
    ]);
  });

  it("shows what the daemon found: installed, installed but untested, and not installed", () => {
    const restore = useCatalog(M, GOLDEN_CATALOG);
    render(() => <Onboarding />);
    const rows = screen.getAllByRole("listitem").map((row) => row.textContent);
    expect(rows).toEqual([
      "Claude Code2.1.282Found",
      expect.stringMatching(
        /^Gemini CLI0\.36\.0FoundMarshal has not been tested with Gemini CLI 0\.36\.0\./,
      ),
      expect.stringMatching(
        /^CodexNot installedCodex is not installed\. Install it with: npm install -g @openai\/codex$/,
      ),
    ]);
    expect(screen.getByText("Not installed")).toBeVisible();
    restore();
  });

  it("does not list the built-in agent as found, because it is Marshal's own", () => {
    render(() => <Onboarding />);
    expect(screen.queryByText("Built-in agent")).toBeNull();
  });

  it("says Marshal looked, not found, while an agent is missing", () => {
    const restore = useCatalog(M, GOLDEN_CATALOG);
    render(() => <Onboarding />);
    expect(screen.getByText(/^Marshal looked for these agents on this computer\./)).toBeVisible();
    restore();
    cleanup();
    render(() => <Onboarding />);
    expect(screen.getByText(/^Marshal found these agents on this computer\./)).toBeVisible();
  });

  it("lists no agent rows when the daemon reports none", () => {
    const restore = useCatalog(M, wireCatalog([]));
    render(() => <Onboarding />);
    expect(screen.queryAllByRole("listitem")).toEqual([]);
    restore();
  });

  it("shows no version for an agent that has none", () => {
    const restore = useCatalog(
      M,
      wireCatalog([wireAgent({ kind: "codex", name: "Codex", status: "missing", version: "" })]),
    );
    render(() => <Onboarding />);
    expect(screen.getByRole("listitem").querySelector("code")).toBeNull();
    restore();
  });

  it("saves a masked key on Continue", () => {
    render(() => <Onboarding />);
    const field = screen.getByLabelText("Anthropic API key");
    expect(field).toHaveAttribute("type", "password");
    type(field, LONG_KEY);
    click("Continue");
    const provider = M.S.providers.find((p) => p.id === "anthropic");
    expect(provider?.masked).toBe("sk-ant…1234");
  });
});

describe("Onboarding project screen", () => {
  beforeEach(() => goTo(3));

  it("offers a folder or a GitHub clone only, and starts on the folder with its field", () => {
    render(() => <Onboarding />);
    const choices = screen.getAllByRole("radio").map((radio) => radio.textContent);
    expect(choices).toEqual([
      "Pick a folderA repository already on this computer",
      "Clone from GitHubPaste a repository URL",
    ]);
    expect(screen.getByRole("radio", { name: /Pick a folder/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(screen.queryByRole("radio", { name: /sample/i })).toBeNull();
    expect(screen.getByPlaceholderText("~/code/my-repo")).toBeInTheDocument();
    expect(screen.queryByText("Repository URL")).toBeNull();
  });

  it("asks for a folder, or a URL, when that choice is picked", () => {
    render(() => <Onboarding />);
    click(/Pick a folder/, "radio");
    expect(screen.getByPlaceholderText("~/code/my-repo")).toBeInTheDocument();
    click(/Clone from GitHub/, "radio");
    expect(screen.queryByPlaceholderText("~/code/my-repo")).toBeNull();
    expect(screen.getByPlaceholderText("https://github.com/owner/repo")).toBeInTheDocument();
  });

  it("adds the project named by the path on Continue", () => {
    const add = vi.spyOn(M, "addProject").mockResolvedValue({ id: "orbit" });
    render(() => <Onboarding />);
    click(/Pick a folder/, "radio");
    type(screen.getByPlaceholderText("~/code/my-repo"), "~/code/orbit");
    click("Continue");
    expect(add).toHaveBeenCalledWith({ source: "folder", path: "~/code/orbit", name: "orbit" });
  });

  it("adds nothing on Continue while the folder is left empty", () => {
    const add = vi.spyOn(M, "addProject").mockResolvedValue({ id: "orbit" });
    render(() => <Onboarding />);
    click("Continue");
    expect(add).not.toHaveBeenCalled();
  });
});

describe("Onboarding last screen", () => {
  beforeEach(() => goTo(4));

  it("shows the pairing code and both chat apps", () => {
    render(() => <Onboarding />);
    expect(screen.getByLabelText("Pairing code")).toHaveTextContent("4K7-Q2M");
    expect(screen.getByText("Expires in 10 minutes")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect Telegram" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect Discord" })).toBeInTheDocument();
  });

  it("marks an app connected with a toast, and saves it on Open Marshal", () => {
    render(() => <Onboarding />);
    click("Connect Discord");
    expect(screen.queryByRole("button", { name: "Connect Discord" })).toBeNull();
    expect(screen.getByText("Connected")).toBeInTheDocument();
    expect(M.S.toasts.map((toast) => toast.msg)).toContain("Discord connected");
    expect(M.S.integrations.find((x) => x.id === "discord")?.st).toBe("error");
    click("Open Marshal");
    expect(M.S.integrations.find((x) => x.id === "discord")?.st).toBe("connected");
    expect(M.S.onboarding).toBe(false);
    expect(M.S.tour).toEqual({ step: 0 });
  });

  it("Skip finishes without saving the connections", () => {
    render(() => <Onboarding />);
    click("Connect Discord");
    click("Skip");
    expect(M.S.onboarding).toBe(false);
    expect(M.S.integrations.find((x) => x.id === "discord")?.st).toBe("error");
  });
});

describe("Onboarding on a phone", () => {
  it("fills the screen without the card border", () => {
    M.set({ vw: PHONE_WIDTH_PX });
    render(() => <Onboarding />);
    const card = screen.getByRole("dialog").firstElementChild;
    expect(card).not.toHaveClass("rounded-xl");
    expect(card).not.toHaveClass("border");
    expect(screen.getByRole("dialog")).toHaveClass("items-stretch");
  });

  it("is a bordered card centered on a larger screen", () => {
    render(() => <Onboarding />);
    const card = screen.getByRole("dialog").firstElementChild;
    expect(card).toHaveClass("rounded-xl", "border");
    expect(screen.getByRole("dialog")).toHaveClass("items-center");
  });
});
