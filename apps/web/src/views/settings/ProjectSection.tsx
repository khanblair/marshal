import { Button, Checkbox, Field, Input, Select, SettingsSection } from "@marshal/ui";
import { M } from "~/mock";
import { GRID_MIN_220 } from "./auto-fit-grid";
import type { ProjectDraft } from "./use-project-draft";

const kindText = (lang: string | undefined, packages: readonly string[] | undefined): string =>
  lang === "Monorepo" ? `Monorepo with ${(packages ?? []).length} packages` : `${lang} project`;

/** Project settings: name, branch, dev command, repository facts, bypass lock, and remove. */
export function ProjectSection(props: { project: ProjectDraft }) {
  const fields = () => props.project.fields();
  return (
    <SettingsSection
      title="Project settings"
      onSubmit={(event) => {
        event.preventDefault();
        props.project.save();
      }}
      actions={
        <Select
          class="px-2.5! font-semibold"
          aria-label="Project"
          options={M.S.projects.map((p) => ({ value: p.id, label: p.name }))}
          value={props.project.projectId() ?? undefined}
          onChange={(e) => props.project.pick(e.currentTarget.value)}
        />
      }
    >
      <div class={GRID_MIN_220}>
        <Field label="Name">
          <Input
            value={fields().name}
            onInput={(e) => props.project.edit("name", e.currentTarget.value)}
          />
        </Field>
        <Field label="Default branch">
          <Input
            mono
            value={fields().branch}
            onInput={(e) => props.project.edit("branch", e.currentTarget.value)}
          />
        </Field>
        <Field label="Dev command" hint="Runs in a card's worktree for the live preview.">
          <Input
            mono
            value={fields().dev}
            placeholder="pnpm dev"
            onInput={(e) => props.project.edit("dev", e.currentTarget.value)}
          />
        </Field>
      </div>
      <div class="flex flex-col gap-1">
        <span class="font-medium">Repository</span>
        <code class="font-mono text-small text-secondary">
          {props.project.project()?.path ?? ""}
        </code>
        <span class="text-small text-secondary">
          {kindText(props.project.project()?.lang, props.project.project()?.packages)}
        </span>
      </div>
      {/* biome-ignore lint/a11y/noLabelWithoutControl: the checkbox is a component inside the label */}
      <label class="flex items-center gap-2.5 cursor-pointer">
        <Checkbox
          checked={fields().lockBypass}
          onChange={() => props.project.edit("lockBypass", !fields().lockBypass)}
        />
        <span class="flex flex-col">
          <span class="font-medium">Lock bypass permissions</span>
          <span class="text-small text-secondary">
            Nobody can turn on bypass for cards in this project.
          </span>
        </span>
      </label>
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" type="submit" disabled={props.project.unchanged()}>
          Save project
        </Button>
        <Button tone="danger" onClick={props.project.remove}>
          Remove project
        </Button>
      </div>
    </SettingsSection>
  );
}
