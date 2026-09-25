import { ChoiceCard, Icon, type IconNameInput } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M, type Theme } from "~/mock";
import { GRID_MIN_160 } from "./auto-fit-grid";

interface ThemeSpec {
  key: Theme;
  label: string;
  icon: IconNameInput;
  description: string;
}

const THEMES: readonly ThemeSpec[] = [
  { key: "light", label: "Light", icon: "sun", description: "Chalk neutrals" },
  { key: "dark", label: "Dark", icon: "moon", description: "Asphalt neutrals" },
  { key: "system", label: "System", icon: "monitor", description: "Follows your OS setting" },
];

/* Each card previews a light or dark board, whatever the current theme. System is light on the
   left and dark on the right. Class names are written out so Tailwind finds them. */
const LIGHT_HALF = {
  bar: "bg-theme-preview-light-line",
  card: "border-theme-preview-light-line bg-theme-preview-light-card",
  bg: "bg-theme-preview-light-bg",
};
const DARK_HALF = {
  bar: "bg-theme-preview-dark-line",
  card: "border-theme-preview-dark-line bg-theme-preview-dark-card",
  bg: "bg-theme-preview-dark-bg",
};

interface Half {
  bar: string;
  card: string;
  bg: string;
}

function PreviewHalf(props: { half: Half; edge: string }) {
  return (
    <span class={`flex-1 flex flex-col gap-1.5 p-2 ${props.half.bg}`}>
      <span class={`h-2 w-3/5 rounded-2xs ${props.half.bar}`} />
      <span class={`h-5.5 rounded-xs border border-l-3 ${props.half.card} ${props.edge}`} />
    </span>
  );
}

function ThemePreview(props: { theme: Theme }) {
  return (
    <span class="flex h-18 rounded-md overflow-hidden border border-border">
      <PreviewHalf
        half={props.theme === "dark" ? DARK_HALF : LIGHT_HALF}
        edge="border-l-theme-preview-working"
      />
      <PreviewHalf
        half={props.theme === "light" ? LIGHT_HALF : DARK_HALF}
        edge="border-l-theme-preview-needs-you"
      />
    </span>
  );
}

/** The Theme radio group: Light, Dark, and System, each with a small board preview. */
export function ThemeCards() {
  return (
    <div role="radiogroup" aria-label="Theme" class={GRID_MIN_160}>
      <For each={THEMES}>
        {(theme) => (
          <ChoiceCard
            class="gap-2.5"
            selected={M.S.theme === theme.key}
            onClick={() => M.setTheme(theme.key)}
          >
            <ThemePreview theme={theme.key} />
            <span class="flex items-center gap-2 font-semibold">
              <Icon name={theme.icon} size={16} />
              {theme.label}
              <Show when={M.S.theme === theme.key}>
                <span class="ml-auto inline-flex">
                  <Icon name="circle-check" size={16} />
                </span>
              </Show>
            </span>
            <span class="text-small leading-4.5 text-secondary">{theme.description}</span>
          </ChoiceCard>
        )}
      </For>
    </div>
  );
}
