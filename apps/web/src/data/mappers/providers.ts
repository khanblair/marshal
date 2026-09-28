import type { CheckState, TestResult, Provider as WireProvider } from "@marshal/protocol";

/**
 * One thing a connection test looked at, in the shape the Providers row reads. It is the wire's
 * `TestCheck` with the fix hint left out when there is none, because the screens test for it.
 */
export interface ProviderTestCheck {
  name: string;
  state: CheckState;
  message: string;
  fix?: string;
}

/** What the last connection test of a provider found, as a row reads it. */
export interface ProviderTest {
  /** True when no check failed. A warning does not make a result not OK. */
  ok: boolean;
  checks: ProviderTestCheck[];
}

/**
 * One model provider row. It is the wire's `Provider` with the two empty forms dropped (`error`,
 * `local`), because the screens ask `if (provider.local)` and `if (provider.error)` rather than
 * comparing to `false` and `""`.
 *
 * It lives in the data layer, not in `mock/`, so the mapper never depends on the mock;
 * `mock/settings-types.ts` re-exports it for the store's own use.
 */
export interface ProviderRow {
  id: string;
  name: string;
  st: "saved" | "empty" | "invalid";
  masked: string;
  models: string;
  /** One plain sentence saying what to do about a key the last check refused. Only for `invalid`. */
  error?: string;
  /** True for a provider that runs on this machine (Ollama, LM Studio) and takes a URL, not a key. */
  local?: boolean;
  /** The last connection test's result, or absent when the provider was never tested. */
  lastTest?: ProviderTest;
}

/** The list answer of every provider route: the rows, and nothing else the store keeps. */
export interface ProviderRows {
  providers: ProviderRow[];
}

function toCheck(check: TestResult["checks"][number]): ProviderTestCheck {
  return {
    name: check.name,
    state: check.state,
    message: check.message,
    ...(check.fix ? { fix: check.fix } : {}),
  };
}

/** One wire test result as a row reads it. */
export function toProviderTest(result: TestResult): ProviderTest {
  return { ok: result.ok, checks: result.checks.map(toCheck) };
}

/** One wire provider as a row reads it. */
function toProviderRow(provider: WireProvider): ProviderRow {
  return {
    id: provider.id,
    name: provider.name,
    st: provider.st,
    masked: provider.masked,
    models: provider.models,
    ...(provider.error ? { error: provider.error } : {}),
    ...(provider.local ? { local: true } : {}),
    ...(provider.lastTest ? { lastTest: toProviderTest(provider.lastTest) } : {}),
  };
}

/**
 * The whole list answer, in the order the daemon sent it. A change route answers the same shape,
 * so the same function applies a read and a save.
 */
export function toProviderRows(answer: { providers: readonly WireProvider[] }): ProviderRows {
  return { providers: answer.providers.map(toProviderRow) };
}
