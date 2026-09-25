import { Icon } from "@marshal/ui";

const CLOSE_ICON_PX = 16;

export interface DialogHeaderProps {
  /** The heading's id, which the dialog names itself by. */
  id: string;
  title: string;
  onClose: () => void;
}

/** The title row of a form dialog: the heading and a close button. */
export function DialogHeader(props: DialogHeaderProps) {
  return (
    <div class="flex items-center gap-2">
      <h2 id={props.id} class="m-0 flex-1 text-title leading-6 font-semibold">
        {props.title}
      </h2>
      <button
        type="button"
        onClick={() => props.onClose()}
        aria-label="Close"
        class="size-7 inline-flex items-center justify-center p-0 border-none rounded-sm bg-transparent"
      >
        <Icon name="x" size={CLOSE_ICON_PX} />
      </button>
    </div>
  );
}
