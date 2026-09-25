import { describe, expect, it } from "vitest";
import { MaxMessageChars, type SendMessageRequest } from "../src";
import { golden } from "./golden";

// The sample is checked against the generated type by the compiler. If a field of the request
// changes in Go, its golden file changes, and this stops compiling until the sample matches.
describe("the send message request golden file from the daemon", () => {
  it("has the text of the message", () => {
    const sample: SendMessageRequest = { text: "Please also add a test for the empty case." };
    expect(golden("send-message-request")).toEqual(sample);
  });

  it("shares the length limit with the daemon", () => {
    expect(MaxMessageChars).toBe(100000);
  });
});
