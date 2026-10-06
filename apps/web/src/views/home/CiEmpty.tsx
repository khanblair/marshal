import { Button, EmptyState, NotConnected } from "@marshal/ui";
import { Show } from "solid-js";
import { M } from "~/mock";
import { openIntegrations } from "./home-actions";

const GITHUB_ID = "github";
const githubConnected = (): boolean =>
  M.S.integrations.find((row) => row.id === GITHUB_ID)?.st === "connected";

/**
 * What Home's CI health and its "all" page show while no project has CI data. The daemon leaves a
 * project out until it has a run for it, so the cause is one of two: GitHub is not connected, or it
 * is and no run has been reported yet (runs arrive by webhook and from the branches of cards).
 * Each says which it is. It draws no rows and no state, only what is missing.
 */
export function CiEmpty() {
  return (
    <Show
      when={githubConnected()}
      fallback={
        <NotConnected
          service="GitHub"
          reason="CI runs appear here once GitHub is connected."
          onConnect={openIntegrations}
        />
      }
    >
      <EmptyState
        icon="circle-dashed"
        messageClass="max-w-[40ch]"
        action={
          <Button variant="primary" onClick={openIntegrations}>
            Open GitHub settings
          </Button>
        }
      >
        GitHub is connected, but no CI run has been reported yet. Marshal hears about runs from
        GitHub webhooks and from the branches of your cards.
      </EmptyState>
    </Show>
  );
}
