import type { GoogleFileKind, GoogleLinkContent } from "@marshal/protocol";
import { oneLine } from "./export-text";

/** Which connection reads each kind of Google address. */
const SERVICE_OF_PRODUCT: Readonly<Record<string, string>> = {
  document: "gdocs",
  spreadsheets: "gsheets",
  presentation: "gslides",
};
const PRODUCT_PATH = /^(?:\/u\/\d+)?\/(document|spreadsheets|presentation)\//;

/** What each kind of file is called in the person's words. */
export const KIND_NAMES: Readonly<Record<GoogleFileKind, string>> = {
  doc: "Doc",
  sheet: "Sheet",
  slides: "Slides",
  file: "File",
};

/**
 * The connection a pasted address needs, judged from the address alone, or null when it is not
 * one of Google's. The daemon makes the real decision, so a null here only skips an early hint.
 */
export function linkService(text: string): string | null {
  let address: URL;
  try {
    address = new URL(text.trim());
  } catch {
    return null;
  }
  if (address.hostname !== "docs.google.com") return null;
  const product = PRODUCT_PATH.exec(address.pathname)?.[1];
  return product ? (SERVICE_OF_PRODUCT[product] ?? null) : null;
}

/** The note with what was read added at the end, under a heading that says where it came from. */
export function noteWithImport(note: string, content: GoogleLinkContent): string {
  const title = oneLine(content.title) || "Untitled";
  const source = content.url ? ["", `Source: ${content.url}`] : [];
  const block = [
    `## From Google ${KIND_NAMES[content.kind]}: ${title}`,
    "",
    content.markdown.trim(),
    ...source,
  ].join("\n");
  const kept = note.trimEnd();
  return kept ? `${kept}\n\n${block}\n` : `${block}\n`;
}
