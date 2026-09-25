import type { ErrorResponse } from "@marshal/protocol";
import { ErrorCodeValues } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { ApiError, clientError, readWireError, wireError } from "./api-error";
import { golden } from "./testing/golden";

const CLIENT_CODES = ["unreachable", "timeout", "aborted", "bad_response"] as const;

describe("clientError", () => {
  it.each(CLIENT_CODES)("gives %s a fixed plain sentence", (code) => {
    const error = clientError(code);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toBeInstanceOf(Error);
    expect(error.code).toBe(code);
    expect(error.status).toBeNull();
    // A sentence a person can read: capital letter, full stop, no code or stack talk.
    expect(error.message).toMatch(/^[A-Z].*\.$/);
    expect(error.message).not.toContain(code);
  });

  it("says the daemon cannot be reached in the words of the design", () => {
    expect(clientError("unreachable").message).toBe(
      "Marshal can't reach the daemon. Check that it is running.",
    );
  });

  it("carries the status of an answer that could not be read", () => {
    expect(clientError("bad_response", 502).status).toBe(502);
  });

  it("marks only unreachable and timeout as worth retrying", () => {
    expect(clientError("unreachable").retryable).toBe(true);
    expect(clientError("timeout").retryable).toBe(true);
    expect(clientError("aborted").retryable).toBe(false);
    expect(clientError("bad_response").retryable).toBe(false);
  });
});

describe("wireError", () => {
  const { error } = golden<ErrorResponse>("error");

  it("keeps the message, code, and details of the daemon", () => {
    const made = wireError(404, error);
    expect(made.code).toBe("not_found");
    expect(made.status).toBe(404);
    expect(made.message).toBe(error.message);
    expect(made.details).toEqual({ id: "01M3C107JB041061050R3GG28A" });
  });

  it.each([
    ["unavailable", true],
    ["internal", true],
    ["not_found", false],
    ["unauthorized", false],
    ["conflict", false],
    ["invalid_argument", false],
  ] as const)("marks %s as retryable: %s", (code, retryable) => {
    expect(wireError(500, { code, message: "x" }).retryable).toBe(retryable);
  });
});

describe("readWireError", () => {
  it("reads the golden error", () => {
    const { error } = golden<ErrorResponse>("error");
    expect(readWireError(error)).toEqual(error);
  });

  it("accepts every code the daemon lists", () => {
    for (const code of ErrorCodeValues) {
      expect(readWireError({ code, message: "A sentence." })).toEqual({
        code,
        message: "A sentence.",
      });
    }
  });

  it.each([
    ["null", null],
    ["a string", "nope"],
    ["an array", []],
    ["no code", { message: "x" }],
    ["an unknown code", { code: "teapot", message: "x" }],
    ["no message", { code: "internal" }],
    ["an empty message", { code: "internal", message: "" }],
    ["a message that is not text", { code: "internal", message: 4 }],
  ])("refuses %s", (_name, value) => {
    expect(readWireError(value)).toBeNull();
  });

  it("drops details that are not all text", () => {
    expect(readWireError({ code: "internal", message: "x", details: { id: 4 } })).toEqual({
      code: "internal",
      message: "x",
    });
    expect(readWireError({ code: "internal", message: "x", details: "no" })).toEqual({
      code: "internal",
      message: "x",
    });
  });
});
