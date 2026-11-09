import { createMemo, onCleanup } from "solid-js";
import { renderMarkdown } from "./render";

export interface MarkdownProps {
  /** The markdown to show. It is rendered as safe HTML, never as the page's own markup. */
  text: string;
  class?: string;
  onLinkClick?: (url: string, event: MouseEvent) => boolean | void;
}

/** Shows markdown text. The styles are the `.md` rules in styles/app.css. */
export function Markdown(props: MarkdownProps) {
  const html = createMemo(() => renderMarkdown(props.text));
  const handleClick = (event: MouseEvent) => {
    const target = event.target;
    if (!(target instanceof HTMLElement)) return;
    const anchor = target.closest("a[href]");
    const href = anchor?.getAttribute("href");
    if (href && props.onLinkClick?.(href, event) === true) event.preventDefault();
  };

  return (
    <div
      class={`md ${props.class ?? ""}`}
      innerHTML={html()}
      ref={(element) => {
        element.addEventListener("click", handleClick);
        onCleanup(() => element.removeEventListener("click", handleClick));
      }}
    />
  );
}
