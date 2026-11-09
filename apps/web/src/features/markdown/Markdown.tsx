import { createMemo } from "solid-js";
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
  return (
    <div
      class={`md ${props.class ?? ""}`}
      innerHTML={html()}
      onClick={(event) => {
        const anchor = (event.target as HTMLElement).closest("a[href]");
        const href = anchor?.getAttribute("href");
        if (href && props.onLinkClick?.(href, event) === true) event.preventDefault();
      }}
    />
  );
}
