import { errorAnswer, type FakeRequest } from "~/data/testing/fake-fetch";

/** The statuses the fake daemon answers with. */
export const STATUS = {
  created: 201,
  badRequest: 400,
  unauthorized: 401,
  notFound: 404,
  conflict: 409,
};
const MAX_ID_CHARS = 24;

export const pathOf = (url: string): string => url.replace(/^https?:\/\/[^/]+/, "");
export const keyOf = (request: FakeRequest): string => `${request.method} ${pathOf(request.url)}`;

export const slug = (name: string): string =>
  name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, MAX_ID_CHARS) || "project";

export const baseName = (path: string): string => path.replace(/\/+$/, "").split("/").pop() ?? "";

/** Reads the JSON body of a request. A body that is not JSON is `{}`, which the routes then refuse. */
export function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/** Tells the event stream about a change, as the daemon does after it. */
export type Publish = (topic: string, type: string, data: unknown) => void;

export const refuse = (status: number, code: string, message: string) =>
  errorAnswer(status, code, message);
