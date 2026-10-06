import type { GoogleFileKind } from "@marshal/protocol";
import { GDOCS_ID, GDRIVE_ID, GSHEETS_ID, GSLIDES_ID } from "~/sync/integrations";

/** What differs between Google Drive, Docs, Sheets and Slides: the rest of their dialog is shared. */
export interface GoogleFileSpec {
  id: string;
  name: string;
  /** The kind of file its Files tab lists. Drive lists every kind Marshal makes. */
  kind?: GoogleFileKind;
  /** One plain sentence on what the connection is for. */
  purpose: string;
  /** One plain sentence on what Google will ask to allow, and what Marshal will not do. */
  permission: string;
}

const AND_NEW_FILES = "and make new files in your Drive";

export const GOOGLE_FILE_SPECS: readonly GoogleFileSpec[] = [
  {
    id: GDRIVE_ID,
    name: "Google Drive",
    purpose: "Marshal saves the notes and files you choose in a folder in your Google Drive.",
    permission:
      "Google will ask to let Marshal make files in your Drive and open the ones it made. It cannot see or change your other files.",
  },
  {
    id: GDOCS_ID,
    name: "Google Docs",
    kind: "doc",
    purpose:
      "Marshal turns a card into a Google Doc, and reads a Google Doc when you give it the link.",
    permission: `Google will ask to let Marshal read your Google Docs ${AND_NEW_FILES}. It never changes or deletes a document you already had.`,
  },
  {
    id: GSHEETS_ID,
    name: "Google Sheets",
    kind: "sheet",
    purpose:
      "Marshal turns a board into a Google Sheet, and reads a Google Sheet when you give it the link.",
    permission: `Google will ask to let Marshal read your Google Sheets ${AND_NEW_FILES}. It never changes or deletes a spreadsheet you already had.`,
  },
  {
    id: GSLIDES_ID,
    name: "Google Slides",
    kind: "slides",
    purpose:
      "Marshal turns a board into a Google Slides presentation, and reads one when you give it the link.",
    permission: `Google will ask to let Marshal read your Google Slides ${AND_NEW_FILES}. It never changes or deletes a presentation you already had.`,
  },
];

/** The spec of a connection by its id, or undefined for one that is not a Google file connection. */
export const googleFileSpec = (id: string): GoogleFileSpec | undefined =>
  GOOGLE_FILE_SPECS.find((spec) => spec.id === id);
