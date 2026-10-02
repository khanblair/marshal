import { Button, Dialog, SegmentedControl, type SegmentOption } from "@marshal/ui";
import { createSignal, Match, Switch } from "solid-js";
import type { Integration } from "~/mock";
import { M } from "~/mock";
import type { EditState } from "./edit-state";
import { GitHubSignIn } from "./GitHubSignIn";
import { GitHubTokenForm } from "./GitHubTokenForm";
import { createGitHubConnect } from "./github-connect";

type Way = "sign-in" | "token";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "sign-in", label: "Sign in with GitHub" },
  { value: "token", label: "Paste a token" },
];

/**
 * Connecting GitHub (section S29a), as a dialog over Settings: a sign-in with GitHub's own device
 * flow, or a personal access token. Both tabs read the one connection, so what one connects, the
 * other shows. Closing it, by any way, ends a sign-in that is still waiting on GitHub.
 */
export function GitHubConnectDialog(props: { integration: Integration; edit: EditState }) {
  const [way, setWay] = createSignal<Way>("sign-in");
  const github = createGitHubConnect();
  const close = (): void => props.edit.close();
  return (
    <Dialog width={560} phone={M.mobile} aria-labelledby="github-title" onClose={close}>
      <h2 id="github-title" class="m-0 text-title leading-6 font-semibold">
        Connect GitHub
      </h2>
      <SegmentedControl
        kind="tabs"
        size={30}
        fill
        unselectedTone="primary"
        label="How to connect GitHub"
        options={WAYS}
        value={way()}
        onValueChange={setWay}
      />
      <div role="tabpanel" aria-label={WAYS.find((option) => option.value === way())?.label}>
        <Switch>
          <Match when={way() === "sign-in"}>
            <GitHubSignIn integration={props.integration} github={github} />
          </Match>
          <Match when={way() === "token"}>
            <GitHubTokenForm integration={props.integration} github={github} />
          </Match>
        </Switch>
      </div>
      <div class="flex justify-end gap-2">
        <Button onClick={close}>Close</Button>
      </div>
    </Dialog>
  );
}
