import DOMPurify from "dompurify";
import { Marked } from "marked";

/** Only these tags survive. No images, scripts, styles, iframes, forms, or raw HTML of any kind. */
const ALLOWED_TAGS = [
  "p",
  "br",
  "strong",
  "em",
  "del",
  "code",
  "pre",
  "blockquote",
  "ul",
  "ol",
  "li",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "a",
  "hr",
  "table",
  "thead",
  "tbody",
  "tr",
  "th",
  "td",
  "input",
];
const ALLOWED_ATTR = ["href", "title", "align", "type", "checked", "disabled"];
/** A link may go to a web page, a mail address, or a place on the same page. Nothing else. */
const ALLOWED_URI = /^(?:https?:|mailto:|#)/i;

const ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};
/** Text with the five characters that mean something in HTML written as plain text. */
export const escapeHtml = (text: string): string =>
  text.replace(/[&<>"']/g, (c) => ESCAPES[c] ?? c);

/**
 * Agent output is untrusted, so the parser is told not to pass HTML through (a tag an agent writes
 * about, like <div>, shows as text) and an image becomes a plain link, which sends nothing until it
 * is clicked. A single newline is a line break, as it was when the text was shown as it came.
 */
const parser = new Marked({
  gfm: true,
  breaks: true,
  async: false,
  renderer: {
    html: ({ text }) => escapeHtml(text),
    image: ({ href, text }) => `<a href="${escapeHtml(href)}">${escapeHtml(text || href)}</a>`,
  },
});

// Every link opens in a new tab and cannot reach back into this page. Task-list boxes are read-only.
DOMPurify.addHook("afterSanitizeAttributes", (node) => {
  if (node.tagName === "A" && node.hasAttribute("href")) {
    node.setAttribute("target", "_blank");
    node.setAttribute("rel", "noopener noreferrer");
  }
  if (node.tagName === "INPUT") {
    node.setAttribute("disabled", "");
    node.setAttribute("type", "checkbox");
  }
});

/** Markdown in, safe HTML out. Never throws: text that cannot be parsed comes back escaped. */
export function renderMarkdown(source: string): string {
  try {
    const html = parser.parse(source) as string;
    return DOMPurify.sanitize(html, {
      ALLOWED_TAGS,
      ALLOWED_ATTR,
      ALLOWED_URI_REGEXP: ALLOWED_URI,
    });
  } catch {
    return escapeHtml(source);
  }
}
