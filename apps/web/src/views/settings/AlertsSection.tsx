import { Checkbox, SettingsPanel, SettingsSection } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { M } from "~/mock";

/** The alert's channels with one added or taken away, in the order they were chosen. */
function toggled(channels: readonly string[], channel: string, on: boolean): string[] {
  const rest = channels.filter((one) => one !== channel);
  return on ? [...rest, channel] : rest;
}

/**
 * Alerts (B9.4, section S26c): for each kind of alert, which channels it is sent to. A channel that
 * is not connected can still be chosen, so a choice made first is kept, but nothing is sent until it
 * is connected under Integrations. The daemon follows a change at once.
 */
export function AlertsSection() {
  return (
    <SettingsSection title="Alerts">
      <p class="m-0 text-secondary">
        Choose where Marshal sends each kind of alert. Connect Telegram, Discord, or ntfy under
        Integrations first.
      </p>
      <Show
        when={M.alerts()}
        fallback={<p class="m-0 text-secondary">Alert settings load from your Marshal computer.</p>}
      >
        {(settings) => (
          <SettingsPanel class="flex flex-col">
            <Index each={settings().routes}>
              {(route) => (
                <fieldset class="flex flex-wrap items-center gap-x-6 gap-y-2 m-0 py-3 px-4 border-0 border-b border-border">
                  <legend class="sr-only">{route().label}</legend>
                  <span class="flex-[1_1_220px] font-semibold">{route().label}</span>
                  <Index each={settings().channels}>
                    {(channel) => (
                      <label class="inline-flex items-center gap-2 min-h-8">
                        <Checkbox
                          checked={route().channels.includes(channel().id)}
                          onChange={(event) =>
                            void M.saveAlertChannels(
                              route().event,
                              toggled(route().channels, channel().id, event.currentTarget.checked),
                            )
                          }
                        />
                        <span>{channel().name}</span>
                        <Show when={!channel().connected}>
                          <span class="text-small text-muted">Not connected</span>
                        </Show>
                      </label>
                    )}
                  </Index>
                </fieldset>
              )}
            </Index>
          </SettingsPanel>
        )}
      </Show>
      <p class="m-0 text-small text-secondary">
        ntfy only receives alerts. To approve something, open the link in the alert or reply in
        Telegram or Discord.
      </p>
    </SettingsSection>
  );
}
