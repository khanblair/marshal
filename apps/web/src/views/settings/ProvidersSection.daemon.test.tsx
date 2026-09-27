// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the provider keys are the daemon's).
import { daemon } from "~/testing/daemon-providers-store";
import type { ProviderList, TestCheck } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toProviderRows } from "~/data/mappers/providers";
import { golden } from "~/data/testing/golden";
import { M } from "~/mock";
import { createProviderStore } from "~/testing/fake-providers";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/** The list the daemon answers with before anything is changed, written by the Go tests. */
const GOLDEN = golden<ProviderList>("provider-list");

/** Puts the daemon and the store back to the golden list, so one test cannot see another's save. */
function resetProviders(): void {
  Object.assign(daemon.providers, createProviderStore());
  daemon.calls.length = 0;
  M.set({ settingsSection: "providers", toasts: [], providers: toProviderRows(GOLDEN).providers });
}

beforeEach(resetProviders);
afterEach(cleanup);

const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const testButtons = () => screen.getAllByRole("button", { name: "Test" });
/** The first Test button, which belongs to the first row that has something stored (Anthropic). */
const testButton = (): HTMLElement => {
  const [first] = testButtons();
  if (!first) throw new Error("no Test button is shown");
  return first;
};
const saveKey = () => fireEvent.click(screen.getByRole("button", { name: "Save key" }));
const typeKey = (label: string, value: string): void => {
  fireEvent.input(screen.getByLabelText(label), { target: { value } });
};

describe("the Provider keys section on the daemon", () => {
  it("shows the daemon's rows, the masked value it answered, and its sentence for a refused key", () => {
    render(() => <SettingsView />);
    expect(screen.getByText("Anthropic")).toBeInTheDocument();
    expect(screen.getByText("DeepSeek")).toBeInTheDocument();
    expect(screen.getByText("OpenRouter")).toBeInTheDocument();
    expect(screen.getByText("Ollama")).toBeInTheDocument();
    expect(screen.getByText("sk-ant-…4f2a")).toBeInTheDocument();
    expect(screen.getByText("sk-or-…0b33")).toBeInTheDocument();
    // A local provider's stored value is its address, and it is shown as it is, not masked.
    expect(screen.getByText("http://localhost:11434")).toBeInTheDocument();
    expect(screen.getByText("Not set")).toBeInTheDocument();
    expect(screen.getByText("Invalid key")).toBeInTheDocument();
    expect(
      screen.getByText(
        "OpenRouter rejected this key. Create a new key at openrouter.ai/keys and paste it here.",
      ),
    ).toBeInTheDocument();
    // Three have something stored and can be tested; the empty one has nothing to test.
    expect(testButtons()).toHaveLength(3);
  });

  it("saves a key on the daemon, shows the mask it answered, and closes the form", async () => {
    render(() => <SettingsView />);
    fireEvent.click(screen.getByRole("button", { name: "Add key" }));
    typeKey("DeepSeek API key", "  sk-deep-abcdefgh1234  ");
    saveKey();
    await waitFor(() => expect(toasts()).toContain("Key saved"));
    expect(daemon.bodies("PUT /v1/providers/deepseek").at(-1)).toEqual({
      key: "sk-deep-abcdefgh1234",
    });
    await waitFor(() => expect(screen.getByText("sk-deep-…1234")).toBeInTheDocument());
    expect(screen.queryByLabelText("DeepSeek API key")).not.toBeInTheDocument();
  });

  it("refuses a key shorter than the design's own minimum, without calling the daemon", () => {
    render(() => <SettingsView />);
    fireEvent.click(screen.getByRole("button", { name: "Add key" }));
    typeKey("DeepSeek API key", "short");
    saveKey();
    expect(
      screen.getByText(
        "This key is too short. Copy the full key from your DeepSeek dashboard and paste it again.",
      ),
    ).toBeInTheDocument();
    expect(daemon.bodies("PUT /v1/providers/deepseek")).toEqual([]);
  });

  it("shows the daemon's sentence for a key it refuses, and keeps what was typed", async () => {
    daemon.providers.refuseKey = (row) =>
      `The ${row.name} key was rejected. Make a new one and paste it here.`;
    render(() => <SettingsView />);
    fireEvent.click(screen.getByRole("button", { name: "Add key" }));
    typeKey("DeepSeek API key", "sk-deep-abcdefgh1234");
    saveKey();
    await waitFor(() =>
      expect(toasts()).toContain(
        "The DeepSeek key was rejected. Make a new one and paste it here.",
      ),
    );
    expect(screen.getByLabelText("DeepSeek API key")).toHaveValue("sk-deep-abcdefgh1234");
    expect(screen.getByText("Not set")).toBeInTheDocument();
  });

  it("runs a test on the daemon and shows every check it looked at", async () => {
    render(() => <SettingsView />);
    fireEvent.click(testButton());
    await waitFor(() => expect(toasts()).toContain("Connection test passed"));
    expect(
      daemon.calls.some(
        (call) => call.method === "POST" && call.url.endsWith("/v1/providers/anthropic/test"),
      ),
    ).toBe(true);
    expect(
      screen.getAllByText(/Anthropic accepted the key and answered a tiny request/).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText(/Anthropic did not say how many requests are left/).length,
    ).toBeGreaterThan(0);
  });

  it("shows the daemon's sentence when a test is asked for again too soon", async () => {
    render(() => <SettingsView />);
    fireEvent.click(testButton());
    await waitFor(() => expect(toasts()).toContain("Connection test passed"));
    fireEvent.click(testButton());
    await waitFor(() =>
      expect(toasts()).toContain(
        "This connection was tested a moment ago. Try again in 5 seconds.",
      ),
    );
  });

  it("marks a row invalid when a test finds a bad key, and shows the fix it gave", async () => {
    daemon.providers.checks = (row): TestCheck[] => [
      {
        name: "API key",
        state: "failed",
        message: `${row.name} rejected the key.`,
        fix: `Create a new ${row.name} key and paste it here.`,
      },
    ];
    render(() => <SettingsView />);
    fireEvent.click(testButton());
    await waitFor(() => expect(toasts()).toContain("Connection test failed"));
    await waitFor(() =>
      expect(screen.getByText("Create a new Anthropic key and paste it here.")).toBeInTheDocument(),
    );
    // OpenRouter was already invalid; the test made Anthropic a second one.
    expect(screen.getAllByText("Invalid key")).toHaveLength(2);
  });

  it("asks a local provider for its address, not a key, and stores the address", async () => {
    render(() => <SettingsView />);
    // Only the local provider takes an address; every other row takes a key.
    expect(screen.getAllByRole("button", { name: "Change URL" })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Change URL" }));
    const field = screen.getByLabelText<HTMLInputElement>("Server URL");
    expect(field).toHaveAttribute("type", "text");
    typeKey("Server URL", "http://127.0.0.1:1234/v1");
    saveKey();
    await waitFor(() => expect(toasts()).toContain("Key saved"));
    expect(daemon.bodies("PUT /v1/providers/ollama").at(-1)).toEqual({
      key: "http://127.0.0.1:1234/v1",
    });
    await waitFor(() => expect(screen.getByText("http://127.0.0.1:1234/v1")).toBeInTheDocument());
  });
});
