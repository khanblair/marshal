import { Button } from "@marshal/ui";
import { For, type JSX, Match, Show, Switch } from "solid-js";
import type {
  GitHubAccount,
  GitHubConnection,
  GitHubConnectState,
} from "~/data/mappers/integrations";
import type { Integration } from "~/mock";
import { GitHubStored } from "./GitHubStored";
import type { GitHubConnectController } from "./github-connect";

/** What a sign-in that ended without connecting says when the daemon gave no sentence of its own. */
const ENDED: Partial<Record<GitHubConnectState, string>> = {
  denied: "You refused the code on GitHub, so nothing was connected.",
  expired: "The code ran out before it was approved.",
  failed: "The sign-in did not finish.",
};

function Note(props: { children: JSX.Element }) {
  return <p class="m-0 text-small leading-4.5 text-secondary">{props.children}</p>;
}

/** An address shown as text, so it can be typed in a browser where no button could open it. */
function Address(props: { label: string; url: string }) {
  return (
    <span class="text-small leading-4.5 text-secondary">
      {props.label} <span class="font-mono select-all break-all">{props.url}</span>
    </span>
  );
}

/** For a person whose browser is signed in to the wrong GitHub account: start the sign-in over. */
function WrongAccount(props: { login?: string; disabled: boolean; onSwitch: () => void }) {
  return (
    <>
      <Note>
        {props.login ? `Not @${props.login}?` : "Not your account?"} Sign out of GitHub in your
        browser, or switch accounts there, then start again.
      </Note>
      <div class="flex flex-wrap gap-2">
        <Button disabled={props.disabled} onClick={props.onSwitch}>
          Switch account
        </Button>
      </div>
    </>
  );
}

function accountLine(entry: GitHubAccount): string {
  return `@${entry.account}, ${entry.allRepositories ? "all" : "selected"} repositories`;
}

/**
 * The first tab: GitHub's own sign-in. The daemon asks GitHub for a code, the person types it on
 * GitHub, and the dialog follows along by itself. One panel for each state the daemon reports.
 */
export function GitHubSignIn(props: { integration: Integration; github: GitHubConnectController }) {
  const github = () => props.github;
  const at = (state: GitHubConnectState): GitHubConnection | undefined => {
    const connection = github().connection();
    return connection?.state === state ? connection : undefined;
  };
  const connectedByToken = () => at("connected")?.mode === "token";
  const signedIn = (): GitHubConnection | undefined => {
    const connection = at("connected");
    return connection?.mode === "token" ? undefined : connection;
  };
  const ended = (): GitHubConnection | undefined => at("denied") ?? at("expired") ?? at("failed");
  const signInButton = (label: string): JSX.Element => (
    <Button variant="primary" disabled={github().starting()} onClick={() => github().signIn()}>
      {label}
    </Button>
  );
  return (
    <div class="flex flex-col gap-3">
      <Switch>
        <Match when={at("pending")}>
          {(connection) => (
            <>
              <p class="m-0">
                Type this code on GitHub and approve. This screen updates by itself.
              </p>
              <Show when={connection().userCode}>
                {(code) => (
                  <code class="self-start font-mono text-display leading-8 font-semibold px-3 py-1 rounded-sm bg-surface-sunken select-all">
                    {code()}
                  </code>
                )}
              </Show>
              <div class="flex flex-wrap gap-2">
                <Show when={connection().verificationUri}>
                  {(url) => (
                    <Button variant="primary" onClick={() => github().open(url())}>
                      Open GitHub
                    </Button>
                  )}
                </Show>
                <Show when={connection().userCode}>
                  {(code) => <Button onClick={() => github().copyCode(code())}>Copy code</Button>}
                </Show>
              </div>
              <Show when={connection().verificationUri}>
                {(url) => <Address label="Or open" url={url()} />}
              </Show>
              <Note>
                Before you approve, check that GitHub shows the account that owns your repositories.
                If it shows another account, switch accounts on github.com first.
              </Note>
            </>
          )}
        </Match>
        <Match when={at("needs_install")}>
          {(connection) => (
            <>
              <p class="m-0">
                {connection().login ? `Signed in as @${connection().login}.` : "Signed in."} Install
                Marshal Kanban on your account, choosing All repositories.
              </p>
              <Show when={connection().installUrl}>
                {(url) => (
                  <>
                    <div class="flex flex-wrap gap-2">
                      <Button variant="primary" onClick={() => github().open(url())}>
                        Install Marshal Kanban
                      </Button>
                    </div>
                    <Address label="Or open" url={url()} />
                  </>
                )}
              </Show>
              <Note>This screen updates by itself once it is installed.</Note>
              <Note>
                On an organization you do not own, an owner has to approve the install first.
              </Note>
              <WrongAccount
                login={connection().login}
                disabled={github().starting()}
                onSwitch={() => github().signIn()}
              />
            </>
          )}
        </Match>
        <Match when={signedIn()}>
          {(connection) => (
            <>
              <p class="m-0 font-semibold">
                {connection().login ? `Connected as @${connection().login}` : "Connected"}
              </p>
              <Show when={connection().installations.length > 0}>
                <ul class="m-0 pl-4 flex flex-col gap-0.5">
                  <For each={connection().installations}>
                    {(entry) => <li class="leading-5">{accountLine(entry)}</li>}
                  </For>
                </ul>
              </Show>
              <GitHubStored
                integration={props.integration}
                onDisconnected={() => void github().refresh()}
              >
                <Show when={connection().installUrl}>
                  {(url) => (
                    <Button onClick={() => github().addAccount(url())}>
                      Add an account or organization
                    </Button>
                  )}
                </Show>
              </GitHubStored>
              <Note>Saving a token on the other tab replaces this sign-in.</Note>
              <WrongAccount
                login={connection().login}
                disabled={github().starting()}
                onSwitch={() => github().signIn()}
              />
            </>
          )}
        </Match>
        <Match when={ended()}>
          {(connection) => (
            <>
              <p class="m-0">{connection().message ?? ENDED[connection().state]}</p>
              <div class="flex flex-wrap gap-2">{signInButton("Try again")}</div>
            </>
          )}
        </Match>
        <Match when={true}>
          <p class="m-0">
            Sign in with your GitHub account. Marshal opens GitHub, shows a code, and does the rest.
          </p>
          <Show when={connectedByToken()}>
            <Note>A personal access token is connected now. Signing in replaces it.</Note>
          </Show>
          <div class="flex flex-wrap gap-2">{signInButton("Sign in with GitHub")}</div>
        </Match>
      </Switch>
      <Show when={github().error()}>
        {(message) => (
          <span role="alert" class="text-small leading-4.5 text-status-danger-text">
            {message()}
          </span>
        )}
      </Show>
    </div>
  );
}
