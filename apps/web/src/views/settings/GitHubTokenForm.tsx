import { Button, Field, Input } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import type { Integration, ProviderTest } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import type { GitHubConnectController } from "./github-connect";
import { saveGitHubToken, testGitHubToken } from "./integration-actions";
import { TestChecks } from "./TestChecks";

const TOKEN_HELP =
  "Use a classic token with the repo scope, or a fine-grained token with read and write access to Contents, Pull requests, Issues and Actions, and read access to Checks. Marshal keeps it in the OS keychain.";
const NO_TOKEN = "Paste a GitHub token first.";

/** The one sentence a stateless test is summed up in: what failed and how to fix it, or what passed. */
function summaryOf(test: ProviderTest): string {
  const failed = test.checks.find((check) => check.state === "failed");
  if (failed) return failed.fix ?? failed.message;
  const summary = test.checks.find((check) => check.name === "Summary");
  return summary?.message ?? (test.ok ? "The token works." : "The token did not pass the test.");
}

/**
 * The second tab: a personal access token pasted by hand. Test asks the daemon whether GitHub
 * accepts it and saves nothing; Save keeps it in the OS keychain and the daemon tests it again.
 */
export function GitHubTokenForm(props: {
  integration: Integration;
  github: GitHubConnectController;
}) {
  const [token, setToken] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  const [result, setResult] = createSignal<ProviderTest | null>(null);
  const connection = () => props.github.connection();
  const connected = () => connection()?.state === "connected";
  const byToken = () => connected() && connection()?.mode === "token";

  const guarded = (run: (value: string) => Promise<void>): void => {
    const value = token().trim();
    if (!value) {
      setError(NO_TOKEN);
      return;
    }
    setError("");
    setResult(null);
    setBusy(true);
    void run(value).finally(() => setBusy(false));
  };

  const test = (): void =>
    guarded(async (value) => {
      const answer = await testGitHubToken(value);
      if ("error" in answer) setError(answer.error);
      else setResult(answer);
    });

  const save = (): void =>
    guarded(async (value) => {
      const answer = await saveGitHubToken(value);
      if ("error" in answer) {
        setError(answer.error);
        return;
      }
      setToken("");
      await props.github.refresh();
    });

  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
    >
      <Show when={byToken()}>
        <p class="m-0 font-semibold">
          {connection()?.login
            ? `Connected as @${connection()?.login} with a personal access token`
            : "Connected with a personal access token"}
        </p>
        <ConnectionStored
          integration={props.integration}
          onDisconnected={() => void props.github.refresh()}
        />
      </Show>
      <Field label={byToken() ? "Replace token" : "Personal access token"} hint={TOKEN_HELP}>
        <Input
          mono
          type="password"
          name="token"
          autocomplete="off"
          spellcheck={false}
          value={token()}
          invalid={!!error()}
          onInput={(event) => {
            setToken(event.currentTarget.value);
            setResult(null);
            setError("");
          }}
        />
      </Field>
      <Show when={error()}>
        <span role="alert" class="text-small leading-4.5 text-status-danger-text">
          {error()}
        </span>
      </Show>
      <Show when={result()}>
        {(found) => (
          <>
            <span role="status" class="text-small leading-4.5 text-secondary">
              {summaryOf(found())}
            </span>
            <TestChecks test={found()} />
          </>
        )}
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button disabled={busy()} onClick={test}>
          Test
        </Button>
        <Button variant="primary" type="submit" disabled={busy()}>
          Save
        </Button>
      </div>
      <Show when={connected()}>
        <span class="text-small leading-4.5 text-secondary">
          {byToken()
            ? "Saving a new token replaces this one."
            : "GitHub is connected by signing in now. Saving a token replaces that."}
        </span>
      </Show>
    </form>
  );
}
