// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the pairing step is the daemon's).
import { daemon } from "~/testing/daemon-person-store";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import * as platformModule from "~/platform";
import { ControlStep } from "./ControlStep";
import { initialDraft } from "./onboarding-draft";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * Onboarding's last screen once the daemon owns it (section S31b): a real pairing code and the real
 * connections, where the mock drew a fixed code and switches that only marked a row.
 */

const stepProps = () =>
  ({ draft: initialDraft(), setDraft: vi.fn() }) as unknown as Parameters<typeof ControlStep>[0];
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);

beforeEach(() => M.set({ toasts: [] }));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the connect-from-anywhere step on the daemon", () => {
  it("asks the daemon for a pairing code and shows it, with when it stops working", async () => {
    render(() => <ControlStep {...stepProps()} />);
    expect(await screen.findByLabelText("Pairing code")).toHaveTextContent("7QX-2LD");
    expect(daemon.routes()).toContain("POST /v1/me/devices/pairing-code");
    expect(screen.getByText(/Expires in \d+ minutes/)).toBeInTheDocument();
    expect(screen.queryByText("marshal-laptop.tail3f2a.ts.net")).toBeNull();
    expect(screen.queryByRole("img", { name: "Pairing QR code" })).toBeNull();
  });

  it("makes a new code when asked", async () => {
    render(() => <ControlStep {...stepProps()} />);
    await screen.findByLabelText("Pairing code");
    const before = daemon.routes().filter((r) => r === "POST /v1/me/devices/pairing-code").length;
    fireEvent.click(screen.getByRole("button", { name: "New code" }));
    await waitFor(() =>
      expect(daemon.routes().filter((r) => r === "POST /v1/me/devices/pairing-code")).toHaveLength(
        before + 1,
      ),
    );
  });

  it("offers Telegram, Discord, and ntfy, and opens the real form for the one chosen", async () => {
    render(() => <ControlStep {...stepProps()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Connect ntfy" }));
    expect(screen.getByLabelText(/^Topic/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect Telegram" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect Discord" })).toBeInTheDocument();
  });

  it("saves ntfy on the daemon with the topic typed, and says it is connected", async () => {
    render(() => <ControlStep {...stepProps()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Connect ntfy" }));
    fireEvent.input(screen.getByLabelText(/^Topic/), { target: { value: "marshal-7f3a9c" } });
    fireEvent.click(screen.getByRole("button", { name: "Save connection" }));
    await waitFor(() => expect(toasts()).toContain("ntfy connected"));
    expect(daemon.bodies("PUT /v1/integrations/ntfy").at(-1)).toEqual({
      topic: "marshal-7f3a9c",
      server: "",
      token: "",
    });
    expect(await screen.findByText("Connected")).toBeInTheDocument();
  });

  it("finds the Telegram chat from the token, fills it in, and saves nothing", async () => {
    render(() => <ControlStep {...stepProps()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Connect Telegram" }));
    fireEvent.input(screen.getByLabelText(/^Bot token/), { target: { value: "123:TESTTOKEN" } });
    fireEvent.click(screen.getByRole("button", { name: "Find my chat" }));
    await waitFor(() => expect(screen.getByLabelText(/^Chat/)).toHaveValue("777"));
    expect(screen.getByRole("status")).toHaveTextContent("Found Ada Okafor (private)");
    expect(daemon.bodies("POST /v1/integrations/telegram/detect-chat").at(-1)).toEqual({
      token: "123:TESTTOKEN",
    });
    expect(daemon.routes()).not.toContain("PUT /v1/integrations/telegram");
  });

  it("says what to do when nobody has written to the Telegram bot yet", async () => {
    daemon.integrations.detectedChat = {
      found: false,
      chatId: "",
      name: "",
      kind: "",
      message: "Nobody has written to the bot yet. Open your bot in Telegram, press Start.",
    };
    render(() => <ControlStep {...stepProps()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Connect Telegram" }));
    fireEvent.input(screen.getByLabelText(/^Bot token/), { target: { value: "123:TESTTOKEN" } });
    fireEvent.click(screen.getByRole("button", { name: "Find my chat" }));
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Nobody has written to the bot yet",
    );
    expect(screen.getByLabelText(/^Chat/)).toHaveValue("");
  });

  it("shows the daemon's sentence beside the form when Telegram does not accept the token", async () => {
    daemon.integrations.detectRefusal =
      "Telegram did not accept that token. Copy it again from @BotFather.";
    render(() => <ControlStep {...stepProps()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Connect Telegram" }));
    fireEvent.input(screen.getByLabelText(/^Bot token/), { target: { value: "123:WRONG" } });
    fireEvent.click(screen.getByRole("button", { name: "Find my chat" }));
    expect(await screen.findByText(/Telegram did not accept that token/)).toBeInTheDocument();
    daemon.integrations.detectRefusal = undefined;
    // A second try clears the old sentence.
    fireEvent.click(screen.getByRole("button", { name: "Find my chat" }));
    await waitFor(() =>
      expect(screen.queryByText(/Telegram did not accept that token/)).toBeNull(),
    );
  });

  it("asks for the token before it looks for a chat", async () => {
    render(() => <ControlStep {...stepProps()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Connect Telegram" }));
    const asked = () =>
      daemon.routes().filter((route) => route === "POST /v1/integrations/telegram/detect-chat")
        .length;
    const before = asked();
    fireEvent.click(screen.getByRole("button", { name: "Find my chat" }));
    expect(await screen.findByText(/Paste the bot's token first/)).toBeInTheDocument();
    expect(asked()).toBe(before);
  });

  it("says the phone is already paired, and offers no code, in the phone app", async () => {
    vi.spyOn(platformModule, "platform").mockReturnValue({
      ...platformModule.createPlatform("web"),
      kind: "mobile",
    });
    render(() => <ControlStep {...stepProps()} />);
    expect(await screen.findByText("This phone is paired")).toBeInTheDocument();
    expect(screen.queryByLabelText("Pairing code")).toBeNull();
    expect(screen.getByRole("button", { name: "Connect Telegram" })).toBeInTheDocument();
  });
});
