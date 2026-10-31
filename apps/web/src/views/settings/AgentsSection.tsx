import { type Agent, AgentKindBuiltin } from "@marshal/protocol";
import {
  Badge,
  type BadgeProps,
  Button,
  Field,
  Input,
  SettingsPanel,
  SettingsSection,
  Switch,
} from "@marshal/ui";
import { createEffect, createSignal, For, on, Show } from "solid-js";
import { M } from "~/mock";
import { AgentListRow, createTester } from "./agent-rows";

interface AgentBadge {
  label: string;
  icon: string;
  tone: BadgeProps["tone"];
}

/** The state of an agent in one badge: turned off, not installed, found, or found but not tested. */
function badgeOf(agent: Agent): AgentBadge {
  if (agent.off) return { label: "Off", icon: "ban", tone: "neutral" };
  if (agent.status === "missing")
    return { label: "Not installed", icon: "circle-dashed", tone: "neutral" };
  if (agent.status === "untested")
    return { label: "Untested", icon: "triangle-alert", tone: "neutral" };
  return { label: "Found", icon: "check", tone: "working" };
}

/**
 * The path field and the on switch under an agent. A path is checked by the daemon before it is kept,
 * so the form does not check it itself: the daemon's reason is shown by the save.
 */
function AgentForm(props: {
  agent: Agent;
  saving: boolean;
  save: (path: string, off: boolean) => void;
}) {
  const [path, setPath] = createSignal(props.agent.customPath);
  createEffect(
    on(
      () => props.agent.customPath,
      (saved) => setPath(saved),
    ),
  );
  return (
    <div class="basis-full flex flex-col gap-3">
      <form
        class="flex flex-col gap-1.5"
        onSubmit={(event) => {
          event.preventDefault();
          props.save(path().trim(), props.agent.off);
        }}
      >
        <Field
          label="Program path"
          hint="Leave it empty to use the program Marshal found on this computer."
        >
          <Input
            mono
            autocomplete="off"
            placeholder="Use the program Marshal found"
            value={path()}
            onInput={(event) => setPath(event.currentTarget.value)}
          />
        </Field>
        <div class="flex gap-2">
          <Button variant="primary" type="submit" disabled={props.saving}>
            Save path
          </Button>
          <Show when={props.agent.customPath}>
            <Button disabled={props.saving} onClick={() => props.save("", props.agent.off)}>
              Use the found program
            </Button>
          </Show>
        </div>
      </form>
      <label class="flex items-center gap-2.5">
        <Switch
          label={`${props.agent.name} is on for cards`}
          checked={!props.agent.off}
          onCheckedChange={(on) => props.save(props.agent.customPath, !on)}
        />
        <span class="text-small">On for cards</span>
      </label>
    </div>
  );
}

/**
 * One agent: its state, Test, and the path and on switch under it. It is keyed by the kind and reads
 * its agent from the store, so a save redraws the row without losing its Test result or a typed path.
 */
function AgentSettingsRow(props: { kind: Agent["kind"] }) {
  const agent = () => M.S.agents.find((listed) => listed.kind === props.kind);
  const tester = createTester(() => props.kind);
  const [saving, setSaving] = createSignal(false);
  const save = (path: string, off: boolean): void => {
    setSaving(true);
    void M.saveAgentSettings(props.kind, { path, off }).finally(() => setSaving(false));
  };
  return (
    <Show when={agent()}>
      {(current) => {
        const badge = () => badgeOf(current());
        return (
          <AgentListRow
            class="px-4"
            name={current().name}
            version={current().version}
            tester={tester}
            status={
              <Badge size={22} tone={badge().tone} icon={badge().icon}>
                {badge().label}
              </Badge>
            }
            details={
              <div class="basis-full flex flex-col gap-2">
                <Show when={current().warning}>
                  <span class="text-small leading-4.5 text-secondary">{current().warning}</span>
                </Show>
                <AgentForm agent={current()} saving={saving()} save={save} />
              </div>
            }
          />
        );
      }}
    </Show>
  );
}

/**
 * Agents: the CLI programs Marshal starts for cards, with the program each one uses, its on switch,
 * and Test. The list is the daemon's, and a scan looks for the programs again.
 */
export function AgentsSection() {
  const listed = () => M.S.agents.filter((agent) => agent.kind !== AgentKindBuiltin);
  const [scanning, setScanning] = createSignal(false);
  const [scanned, setScanned] = createSignal(false);
  const scan = (): void => {
    setScanning(true);
    setScanned(false);
    void M.scanAgents()
      .then((ok) => setScanned(ok))
      .finally(() => setScanning(false));
  };
  return (
    <SettingsSection
      title="Agents"
      description="The agent programs Marshal starts for cards. Choose the program to use, turn one off, or test that it starts. A test sends no prompt."
    >
      <div class="flex flex-wrap items-center gap-2.5">
        <Button size={28} icon="scan-search" disabled={scanning()} onClick={scan}>
          {scanning() ? "Scanning…" : "Scan again"}
        </Button>
        <Show when={scanning()}>
          <span role="status" class="flex items-center gap-2 text-small text-secondary">
            <span
              aria-hidden="true"
              class="inline-block size-3.5 rounded-full border-2 border-current border-t-transparent animate-spin"
            />
            Looking for agents on this computer…
          </span>
        </Show>
        <Show when={scanned() && !scanning()}>
          <span role="status" class="text-small text-secondary">
            Scanned just now.
          </span>
        </Show>
      </div>
      <SettingsPanel list>
        <For each={listed().map((agent) => agent.kind)}>
          {(kind) => <AgentSettingsRow kind={kind} />}
        </For>
      </SettingsPanel>
    </SettingsSection>
  );
}
