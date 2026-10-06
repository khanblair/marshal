import type {
  AuthorizeURL,
  CreateGoogleDocRequest,
  CreateGoogleSheetRequest,
  CreateGoogleSlidesRequest,
  GoogleFile,
  GoogleFileKind,
  GoogleFiles,
  GoogleLinkContent,
  ReadGoogleLinkRequest,
  SaveGoogleDriveRequest,
  UploadGoogleFileRequest,
} from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";
import { NO_DAEMON, sentence } from "./google-actions";
import { applyIntegrationList, GDRIVE_ID } from "./integrations";

/*
 * Google Drive, Docs, Sheets and Slides (S29i to S29l) beyond the row: consent, files, creates, read.
 * A refusal is the daemon's own sentence, returned as `error` for the screen, never a toast.
 */

/** The files Marshal made, or the sentence that says why they could not be listed. */
export type GoogleFilesAnswer = { files: GoogleFiles } | { error: string };

/** The file a create made, or the sentence that says why none was made. */
export type GoogleFileAnswer = { file: GoogleFile } | { error: string };

/** A Google file read from a link, or the sentence that says why it could not be read. */
export type GoogleLinkAnswer = { content: GoogleLinkContent } | { error: string };

type Asked<T> = { value: T } | { error: string };

/** Asks the daemon one thing and narrows the answer. Nothing here is a toast. */
async function ask<T>(c: Ctx, call: (api: ApiClient) => Promise<T>): Promise<Asked<T>> {
  const api = c.env.data?.api;
  if (!api) return { error: NO_DAEMON };
  try {
    return { value: await call(api) };
  } catch (error) {
    return { error: sentence(error) };
  }
}

const file = (answer: Asked<GoogleFile>): GoogleFileAnswer =>
  "error" in answer ? answer : { file: answer.value };

/** One Google connection's consent URL, for the owner's own browser. Null when the daemon refused it. */
export async function authorizeGoogleService(c: Ctx, id: string): Promise<AuthorizeURL | null> {
  const answer = await ask(c, (api) => api.authorizeGoogleService(id));
  return "error" in answer ? null : answer.value;
}

/** The files Marshal made, newest first. No kind lists every kind Marshal makes. */
export async function googleFiles(c: Ctx, kind?: GoogleFileKind): Promise<GoogleFilesAnswer> {
  const answer = await ask(c, (api) => api.googleFiles(kind));
  return "error" in answer ? answer : { files: answer.value };
}

/** Saves the name of Marshal's Drive folder and applies the whole connection list the daemon answers. */
export async function saveGoogleDrive(
  c: Ctx,
  body: SaveGoogleDriveRequest,
): Promise<{ saved: true } | { error: string }> {
  const answer = await ask(c, (api) => api.saveIntegration(GDRIVE_ID, body));
  if ("error" in answer) return answer;
  applyIntegrationList(c, answer.value);
  return { saved: true };
}

/** Makes a Google Doc in Marshal's Drive folder. */
export async function createGoogleDoc(
  c: Ctx,
  body: CreateGoogleDocRequest,
): Promise<GoogleFileAnswer> {
  return file(await ask(c, (api) => api.createGoogleDoc(body)));
}

/** Makes a Google Sheet in Marshal's Drive folder. */
export async function createGoogleSheet(
  c: Ctx,
  body: CreateGoogleSheetRequest,
): Promise<GoogleFileAnswer> {
  return file(await ask(c, (api) => api.createGoogleSheet(body)));
}

/** Makes a Google Slides presentation in Marshal's Drive folder. */
export async function createGoogleSlides(
  c: Ctx,
  body: CreateGoogleSlidesRequest,
): Promise<GoogleFileAnswer> {
  return file(await ask(c, (api) => api.createGoogleSlides(body)));
}

/** Saves a plain file to Marshal's Drive folder, as it is. */
export async function uploadGoogleFile(
  c: Ctx,
  body: UploadGoogleFileRequest,
): Promise<GoogleFileAnswer> {
  return file(await ask(c, (api) => api.uploadGoogleFile(body)));
}

/** Reads a Google Doc, Sheet or Slides presentation from its link, as markdown. */
export async function readGoogleLink(
  c: Ctx,
  body: ReadGoogleLinkRequest,
): Promise<GoogleLinkAnswer> {
  const answer = await ask(c, (api) => api.readGoogleLink(body));
  return "error" in answer ? answer : { content: answer.value };
}
