import { Button, Icon, SettingsPanel, SettingsSection } from "@marshal/ui";
import { Show } from "solid-js";
import { updateBusy } from "~/features/updates/update-store";
import { openReleases, updates } from "~/features/updates/updates";

const PERCENT = 100;

const labels = {
  web: "Updates are installed in the desktop app",
  idle: "Check for a newer version",
  disabled: "Updates are not enabled in this build",
  checking: "Checking for updates…",
  current: "You’re up to date",
  available: "A new version is available",
  downloading: "Downloading update…",
  installing: "Installing update…",
  installed: "Update installed",
  restarting: "Restarting Marshal…",
  error: "Couldn’t update Marshal",
} as const;

export function UpdatesSection() {
  const state = updates.state;
  const progress = () =>
    state.total
      ? Math.min(PERCENT, Math.round((state.downloaded / state.total) * PERCENT))
      : undefined;
  return (
    <SettingsSection title="Updates">
      <p class="m-0 text-secondary">Get the latest improvements without leaving Marshal.</p>
      <SettingsPanel class="flex flex-col gap-4 p-4">
        <div class="flex items-start gap-3">
          <Icon name="download" size={20} />
          <div class="min-w-0 flex-1">
            <h3 class="m-0 font-semibold">Marshal</h3>
            <p class="m-0 text-small text-secondary">
              {state.currentVersion ? `Installed version ${state.currentVersion}` : "Web client"}
            </p>
          </div>
        </div>
        <div role="status" aria-live="polite" class="flex flex-col gap-2">
          <span class="font-medium">{labels[state.status]}</span>
          <Show when={state.version}>
            <span class="text-secondary">Version {state.version}</span>
          </Show>
          <Show when={state.status === "web"}>
            <p class="m-0 text-secondary">
              Open Marshal on your computer to check for and install app updates. This browser uses
              the version served by your daemon.
            </p>
          </Show>
          <Show when={state.status === "disabled"}>
            <p class="m-0 text-secondary">
              This build has no signed update feed. Install a release with updates enabled to
              receive future versions here.
            </p>
          </Show>
          <Show when={state.status === "downloading"}>
            <Show
              when={progress() !== undefined}
              fallback={
                <progress class="w-full accent-ink" aria-label="Update download" max={PERCENT} />
              }
            >
              <progress
                class="w-full accent-ink"
                aria-label="Update download"
                value={progress() ?? 0}
                max={PERCENT}
              />
            </Show>
            <span class="text-small text-secondary">
              {progress() === undefined ? "Downloading…" : `${progress()}% downloaded`}
            </span>
          </Show>
          <Show when={state.error}>
            <p role="alert" class="m-0 text-secondary break-words">
              {state.error}
            </p>
          </Show>
        </div>
        <div class="flex flex-wrap gap-2">
          <Show when={!["web", "disabled", "installed", "available"].includes(state.status)}>
            <Button disabled={updateBusy(state.status)} onClick={() => void updates.check()}>
              Check for updates
            </Button>
          </Show>
          <Show when={state.status === "available"}>
            <Button variant="primary" onClick={() => void updates.install()}>
              Download and install
            </Button>
          </Show>
          <Show when={state.status === "installed"}>
            <Button variant="primary" onClick={() => void updates.restart()}>
              Restart Marshal
            </Button>
          </Show>
          <Button onClick={openReleases}>View all releases</Button>
        </div>
        <Show when={state.status === "installed"}>
          <p class="m-0 text-small text-secondary">
            Finish any running work before restarting. Restart also updates the background daemon.
          </p>
        </Show>
      </SettingsPanel>
      <Show when={state.notes}>
        <SettingsPanel class="flex flex-col gap-3 p-4">
          <h3 class="m-0 font-semibold">What’s new</h3>
          <p class="m-0 whitespace-pre-wrap break-words text-secondary">{state.notes}</p>
        </SettingsPanel>
      </Show>
      <p class="m-0 text-small text-secondary">
        The installed desktop app checks on launch and every four hours. You choose when to
        download, install, and restart.
      </p>
    </SettingsSection>
  );
}
