import type { Card, Chat, Project } from "@marshal/protocol";
import { createApiClient } from "./api-client";
import { ApiError } from "./api-error";
import { fakeFetch } from "./testing/fake-fetch";
import { golden } from "./testing/golden";

/** The bearer token every fake call in `api-client.test.ts` and `api-client-routes.test.ts` sends. */
export const TOKEN = "test-token-not-real";

/** A project, a card, and a chat both test files build routes and bodies from, by their golden ids. */
export const project = golden<Project>("project");
export const card = golden<Card>("card");
export const chat = golden<Chat>("chat");

export function make(
  routes: Parameters<typeof fakeFetch>[0],
  extra: Partial<Parameters<typeof createApiClient>[0]> = {},
) {
  const fake = fakeFetch(routes);
  const client = createApiClient({ getToken: () => TOKEN, fetch: fake.fetch, ...extra });
  return { ...fake, client };
}

export async function failureOf(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise;
  } catch (error) {
    if (error instanceof ApiError) return error;
    throw error;
  }
  throw new Error("the call did not fail");
}
