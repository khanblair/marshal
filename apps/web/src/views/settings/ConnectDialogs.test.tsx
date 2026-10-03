import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type Integration, M } from "~/mock";
import { DiscordConnectDialog } from "./DiscordConnectDialog";
import { createEditState } from "./edit-state";
import { GmailConnectDialog } from "./GmailConnectDialog";
import * as actions from "./integration-actions";
import { NtfyConnectDialog } from "./NtfyConnectDialog";
import { TelegramConnectDialog } from "./TelegramConnectDialog";
import { TrelloConnectDialog } from "./TrelloConnectDialog";

vi.mock("./integration-actions", () => ({
  connectTelegram: vi.fn(async () => true),
  detectTelegramChat: vi.fn(),
  connectTrello: vi.fn(async () => true),
  connectDiscord: vi.fn(async () => true),
  connectNtfy: vi.fn(async () => true),
  connectGmail: vi.fn(async () => true),
  testConnection: vi.fn(async () => undefined),
  disconnectConnection: vi.fn(),
}));

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

const row = (id: string, name: string, st: Integration["st"] = "none"): Integration => ({
  id,
  name,
  icon: "plug",
  st,
  detail: "",
});

const tab = (name: string) => screen.getByRole("tab", { name });
/** A field's label also holds its hint, so it is found by how it starts. */
const field = (label: string | RegExp) =>
  screen.getByLabelText(typeof label === "string" ? new RegExp(`^${label}`) : label);
const type = (label: string | RegExp, value: string) =>
  fireEvent.input(field(label), { target: { value } });
const press = (name: string) => fireEvent.click(screen.getByRole("button", { name }));

function open<P extends { edit: ReturnType<typeof createEditState> }>(
  Dialog: (props: P & { integration: Integration }) => import("solid-js").JSX.Element,
  integration: Integration,
  extra: Omit<P, "edit" | "integration"> = {} as Omit<P, "edit" | "integration">,
) {
  const edit = createEditState();
  edit.open(integration.id);
  render(() => (
    <Dialog {...({ integration, edit, ...extra } as P & { integration: Integration })} />
  ));
  return edit;
}

describe("Telegram: two tabs that share the token and the chat", () => {
  const telegram = () => row("telegram", "Telegram");

  it("opens on Find my chat, with the other tab one click away", () => {
    open(TelegramConnectDialog, telegram());
    expect(tab("Find my chat")).toHaveAttribute("aria-selected", "true");
    expect(tab("Enter the chat")).toHaveAttribute("aria-selected", "false");
  });

  it("asks for the token before it looks for a chat", () => {
    const edit = open(TelegramConnectDialog, telegram());
    press("Find my chat");
    expect(edit.errorFor("telegram")).toMatch(/Paste the bot's token first/);
    expect(actions.detectTelegramChat).not.toHaveBeenCalled();
  });

  it("finds the chat, carries it to the other tab, and saves both", async () => {
    vi.mocked(actions.detectTelegramChat).mockResolvedValue({
      found: true,
      chatId: "4242",
      name: "Ann",
      kind: "private",
      message: "",
    });
    open(TelegramConnectDialog, telegram());
    type(/^Bot token/, "123:TESTTOKEN");
    press("Find my chat");
    expect(await screen.findByText(/Found Ann \(private\)/)).toBeInTheDocument();
    fireEvent.click(tab("Enter the chat"));
    expect(field("Chat")).toHaveValue("4242");
    expect(field(/^Bot token/)).toHaveValue("123:TESTTOKEN");
    press("Save connection");
    await waitFor(() =>
      expect(actions.connectTelegram).toHaveBeenCalledWith({
        token: "123:TESTTOKEN",
        chatId: "4242",
      }),
    );
  });

  it("says what to do when nobody has written to the bot yet", async () => {
    vi.mocked(actions.detectTelegramChat).mockResolvedValue({
      found: false,
      chatId: "",
      name: "",
      kind: "",
      message: "Nobody has written to the bot yet.",
    });
    open(TelegramConnectDialog, telegram());
    type(/^Bot token/, "123:TESTTOKEN");
    press("Find my chat");
    expect(await screen.findByText("Nobody has written to the bot yet.")).toBeInTheDocument();
  });

  it("points to the other tab when Save is pressed before a chat was found", () => {
    const edit = open(TelegramConnectDialog, telegram());
    type(/^Bot token/, "123:TESTTOKEN");
    press("Save connection");
    expect(edit.errorFor("telegram")).toMatch(/Press Find my chat first/);
    expect(actions.connectTelegram).not.toHaveBeenCalled();
  });

  it("offers Test and Disconnect on both tabs once connected", () => {
    open(TelegramConnectDialog, row("telegram", "Telegram", "connected"));
    expect(screen.getByRole("button", { name: "Test connection" })).toBeInTheDocument();
    fireEvent.click(tab("Enter the chat"));
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
  });
});

describe("Trello: the connection on one tab, the import and webhook on the other", () => {
  const trello = () => row("trello", "Trello");

  it("offers a link that makes a token for the key typed", () => {
    open(TrelloConnectDialog, trello());
    expect(screen.queryByRole("link", { name: /make a token/ })).toBeNull();
    type("API key", "abc123");
    expect(screen.getByRole("link", { name: /make a token/ })).toHaveAttribute(
      "href",
      expect.stringContaining("key=abc123"),
    );
  });

  it("refuses a secret without its callback URL, from either tab", () => {
    const edit = open(TrelloConnectDialog, trello());
    type("API key", "k");
    type("Token", "t");
    type("Board id", "b");
    fireEvent.click(tab("Import and webhook"));
    type("Webhook secret", "s");
    press("Save connection");
    expect(edit.errorFor("trello")).toBe(
      "Enter both the webhook secret and the callback URL, or leave both empty.",
    );
  });

  it("saves what both tabs hold, in one call", async () => {
    expect(M.S.projects.length).toBeGreaterThan(0);
    open(TrelloConnectDialog, trello());
    type("API key", " k ");
    type("Token", "t");
    type("Board id", "b");
    fireEvent.click(tab("Import and webhook"));
    type("Import list id", "list-1");
    type("Webhook secret", "s");
    type("Callback URL", "https://hooks.example/trello");
    press("Save connection");
    await waitFor(() =>
      expect(actions.connectTrello).toHaveBeenCalledWith({
        apiKey: "k",
        token: "t",
        projectId: M.S.projects[0]?.id,
        boardId: "b",
        newCardListId: "list-1",
        webhookSecret: "s",
        callbackUrl: "https://hooks.example/trello",
      }),
    );
  });
});

describe("Discord: the bot on one tab, the channel on the other", () => {
  const discord = () => row("discord", "Discord");

  it("shows the steps for the bot, then for the channel", () => {
    open(DiscordConnectDialog, discord());
    expect(screen.getByText(/Message Content Intent/)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open the Discord developer portal" }),
    ).toBeInTheDocument();
    fireEvent.click(tab("The channel"));
    expect(screen.getByText(/Copy Channel ID/)).toBeInTheDocument();
  });

  it("asks for each in words, whichever tab is open", () => {
    const edit = open(DiscordConnectDialog, discord());
    press("Save connection");
    expect(edit.errorFor("discord")).toBe("Marshal needs the Discord bot's token to reach it.");
    type("Bot token", "tok");
    press("Save connection");
    expect(edit.errorFor("discord")).toBe("Enter the channel Marshal should send notices to.");
  });

  it("saves the token from one tab and the channel from the other", async () => {
    open(DiscordConnectDialog, discord());
    type("Bot token", " tok ");
    fireEvent.click(tab("The channel"));
    type("Channel id", "9001");
    press("Save connection");
    await waitFor(() =>
      expect(actions.connectDiscord).toHaveBeenCalledWith({ token: "tok", channelId: "9001" }),
    );
  });
});

describe("ntfy: the public server, or one of the person's own", () => {
  const ntfy = () => row("ntfy", "ntfy");

  it("makes a hard-to-guess topic on request", () => {
    open(NtfyConnectDialog, ntfy());
    press("Make me a topic");
    expect((field("Topic") as HTMLInputElement).value).toMatch(/^marshal-[0-9a-z]{12}$/);
  });

  it("saves to ntfy.sh with only the topic, ignoring a server typed on the other tab", async () => {
    open(NtfyConnectDialog, ntfy());
    fireEvent.click(tab("My own server"));
    type("Server", "https://ntfy.example.com");
    type("Access token", "secret");
    fireEvent.click(tab("ntfy.sh"));
    type("Topic", "my-topic");
    press("Save connection");
    await waitFor(() =>
      expect(actions.connectNtfy).toHaveBeenCalledWith({
        topic: "my-topic",
        server: "",
        token: "",
      }),
    );
  });

  it("saves to the person's own server with its token", async () => {
    open(NtfyConnectDialog, ntfy());
    fireEvent.click(tab("My own server"));
    type("Server", "https://ntfy.example.com");
    type("Topic", "my-topic");
    type("Access token", "secret");
    press("Save connection");
    await waitFor(() =>
      expect(actions.connectNtfy).toHaveBeenCalledWith({
        topic: "my-topic",
        server: "https://ntfy.example.com",
        token: "secret",
      }),
    );
  });

  it("asks for a topic, and for the address on the own-server tab", () => {
    const edit = open(NtfyConnectDialog, ntfy());
    press("Save connection");
    expect(edit.errorFor("ntfy")).toBe("Enter the ntfy topic Marshal should send alerts to.");
    fireEvent.click(tab("My own server"));
    type("Topic", "t");
    press("Save connection");
    expect(edit.errorFor("ntfy")).toBe("Enter your server's address, or use the ntfy.sh tab.");
  });
});

describe("Gmail: the label on one tab, Google's access on the other", () => {
  const gmail = (st: Integration["st"] = "none") => row("gmail", "Gmail", st);

  it("asks for a label, then moves on to Google access once it is saved but not granted", async () => {
    const edit = open(GmailConnectDialog, gmail());
    press("Save connection");
    expect(edit.errorFor("gmail")).toBe("Choose the Gmail label Marshal should watch.");
    type("Label", "marshal");
    press("Save connection");
    await waitFor(() => expect(actions.connectGmail).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(tab("Google access")).toHaveAttribute("aria-selected", "true"));
  });

  it("opens Google and then asks the row how it went until the grant lands", async () => {
    const grant = vi.fn(async () => true);
    const refresh = vi.fn(async () => undefined);
    open(GmailConnectDialog, gmail(), { grant, refresh });
    fireEvent.click(tab("Google access"));
    press("Grant access");
    await waitFor(() => expect(grant).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(4100);
    expect(refresh.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("says so, and does not keep asking, when Google could not be started", async () => {
    const refresh = vi.fn(async () => undefined);
    const edit = open(GmailConnectDialog, gmail(), { grant: async () => false, refresh });
    fireEvent.click(tab("Google access"));
    press("Grant access");
    await waitFor(() => expect(edit.errorFor("gmail")).toMatch(/no Google sign-in/));
    await vi.advanceTimersByTimeAsync(6000);
    expect(refresh).not.toHaveBeenCalled();
  });

  it("offers Reconnect, Test and Disconnect once connected", () => {
    open(GmailConnectDialog, gmail("connected"));
    fireEvent.click(tab("Google access"));
    expect(screen.getByRole("button", { name: "Reconnect Gmail" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
  });
});
