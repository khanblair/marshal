import { Icon } from "@marshal/ui";
import { M } from "~/mock";

export interface BypassBannerProps {
  cardId: number;
}

/** A red striped bar above everything while the agent runs with bypass permissions. */
export function BypassBanner(props: BypassBannerProps) {
  return (
    <div
      role="alert"
      class="flex-none relative z-banner flex flex-wrap items-center gap-x-3 gap-y-2 py-2.5 px-4 bg-bypass-bg bg-[image:repeating-linear-gradient(135deg,var(--color-bypass-stripe)_0_10px,transparent_10px_20px)] text-bypass-text"
    >
      <Icon name="shield-alert" size={20} />
      <span class="flex-[1_1_240px] font-semibold [text-shadow:0_1px_0_var(--color-scrim-shadow)]">
        Bypass permissions is on. The agent can run any command in this worktree without asking.
      </span>
      <button
        type="button"
        onClick={() => M.turnOffBypass(props.cardId)}
        class="h-7 px-2.5 rounded-sm border border-white bg-bypass-bg text-white text-small font-semibold"
      >
        Turn off bypass
      </button>
    </div>
  );
}
