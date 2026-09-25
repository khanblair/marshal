import {
  type ErrorCode,
  ErrorCodeInternal,
  ErrorCodeUnavailable,
  ErrorCodeValues,
  type Error as WireError,
} from "@marshal/protocol";
import { isRecord } from "./guards";

/** Reasons that come from the app and not from the daemon: no answer, too slow, cancelled, unreadable. */
export type ClientErrorCode = "unreachable" | "timeout" | "aborted" | "bad_response";

/** Every code an `ApiError` can carry: the daemon's own codes and the client-only ones. */
export type ApiErrorCode = ErrorCode | ClientErrorCode;

/* Fixed sentences for the client-only codes. They follow the writing rules in ui-rules.md: say what
   happened and what to do, with no jargon and no apology. */
const CLIENT_MESSAGES: Record<ClientErrorCode, string> = {
  unreachable: "Marshal can't reach the daemon. Check that it is running.",
  timeout: "The daemon took too long to answer. Try again in a moment.",
  aborted: "The request was cancelled before the daemon answered.",
  bad_response:
    "The daemon sent an answer that Marshal can't read. Try again, and update Marshal if it keeps happening.",
};

/** Codes where the same request may work a moment later. */
const RETRYABLE: ReadonlySet<ApiErrorCode> = new Set<ApiErrorCode>([
  "unreachable",
  "timeout",
  ErrorCodeUnavailable,
  ErrorCodeInternal,
]);

interface ApiErrorInit {
  code: ApiErrorCode;
  message: string;
  /** The HTTP status, or null when there was no answer. */
  status: number | null;
  details?: Readonly<Record<string, string>> | undefined;
}

/**
 * The one error the data layer throws. `message` is always a plain sentence a person can read, so
 * a screen can show it as it is. `code` is what a screen branches on.
 */
export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number | null;
  readonly details: Readonly<Record<string, string>> | undefined;
  readonly retryable: boolean;

  constructor(init: ApiErrorInit) {
    super(init.message);
    this.name = "ApiError";
    this.code = init.code;
    this.status = init.status;
    this.details = init.details;
    this.retryable = RETRYABLE.has(init.code);
  }
}

/** An error the app made up itself, with the fixed sentence for its code. */
export function clientError(code: ClientErrorCode, status: number | null = null): ApiError {
  return new ApiError({ code, message: CLIENT_MESSAGES[code], status });
}

/** An error the daemon sent. It keeps the daemon's own message, unchanged. */
export function wireError(status: number, wire: WireError): ApiError {
  return new ApiError({ code: wire.code, message: wire.message, status, details: wire.details });
}

function readDetails(value: unknown): Record<string, string> | undefined {
  if (!isRecord(value)) return undefined;
  const details: Record<string, string> = {};
  for (const [key, item] of Object.entries(value)) {
    if (typeof item !== "string") return undefined;
    details[key] = item;
  }
  return details;
}

/**
 * Reads the error shape from a value that came off the wire: `{ code, message, details? }`. It
 * returns null when the value is anything else, so the caller can call it a bad answer.
 */
export function readWireError(value: unknown): WireError | null {
  if (!isRecord(value)) return null;
  const { code, message, details } = value;
  const known = ErrorCodeValues.find((candidate) => candidate === code);
  if (known === undefined || typeof message !== "string" || message === "") return null;
  const parsed = readDetails(details);
  return parsed ? { code: known, message, details: parsed } : { code: known, message };
}
