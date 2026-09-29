import { describe, expect, it } from "vitest";
import { emailProblem, nameProblem } from "./profile-validation";

describe("nameProblem", () => {
  it("asks for a name and refuses one that is blank, one letter, all digits, or too long", () => {
    expect(nameProblem("")).toMatch(/Enter your name/);
    expect(nameProblem("   ")).toMatch(/Enter your name/);
    expect(nameProblem("A")).toMatch(/at least 2/);
    expect(nameProblem("1234")).toMatch(/at least one letter/);
    expect(nameProblem("x".repeat(101))).toMatch(/at most 100/);
  });

  it("accepts ordinary names, including non-English ones", () => {
    for (const name of ["Ada Lovelace", "  Jo ", "José Núñez", "王小明", "Amy-Lee O'Neil"]) {
      expect(nameProblem(name)).toBe("");
    }
  });
});

describe("emailProblem", () => {
  it("asks for an email, because it is required", () => {
    expect(emailProblem("")).toMatch(/Enter your email/);
    expect(emailProblem("   ")).toMatch(/Enter your email/);
  });

  it("refuses what is not one plain address", () => {
    for (const email of [
      "ada",
      "ada@",
      "@x.com",
      "ada@x",
      "a b@x.com",
      "Ada <ada@x.com>",
      "a@b@c.com",
      "ada@x..com",
    ]) {
      expect(emailProblem(email)).not.toBe("");
    }
  });

  it("accepts a plain address", () => {
    for (const email of ["ada@example.com", " ada.l+briefs@mail.example.co.uk "]) {
      expect(emailProblem(email)).toBe("");
    }
  });
});
