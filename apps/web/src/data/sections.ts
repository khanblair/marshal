/**
 * Every section of the cutover register in docs/backend-checklist.md section 2.2. A section is a part
 * of the screens that can switch from mock data to the daemon on its own.
 */
export type SectionId =
  | "S1"
  | "S2a"
  | "S2b"
  | "S2c"
  | "S3"
  | "S4"
  | "S5a"
  | "S5b"
  | "S5c"
  | "S6a"
  | "S7a"
  | "S7b"
  | "S7c"
  | "S8a"
  | "S8b"
  | "S8c"
  | "S9"
  | "S10"
  | "S11"
  | "S12"
  | "S13"
  | "S14"
  | "S15"
  | "S16"
  | "S17"
  | "S18"
  | "S19a"
  | "S19b"
  | "S20"
  | "S21"
  | "S22"
  | "S23"
  | "S24a"
  | "S24b"
  | "S25"
  | "S26a"
  | "S26b"
  | "S26c"
  | "S27"
  | "S28"
  | "S29a"
  | "S29b"
  | "S29c"
  | "S29d"
  | "S29e"
  | "S29f"
  | "S29g"
  | "S29h"
  | "S30"
  | "S31a"
  | "S31b"
  | "S32";

export type SectionStatus = "mock" | "daemon";

/**
 * Which sections are on the daemon. The register in docs/backend-checklist.md section 2.2 is the
 * one place that says so, and a test fails when this table and that register differ, so a section
 * is switched in the same change that updates the register. S1 (connection and sign-in), S3 (projects), S4 (agents and models), the limits (S26b), the provider keys (S28), the roles (S27), and the plans (S8c) are on the daemon.
 */
export const sectionStatus: Readonly<Record<SectionId, SectionStatus>> = {
  S1: "daemon",
  S2a: "daemon",
  S2b: "daemon",
  S2c: "mock",
  S3: "daemon",
  S4: "daemon",
  S5a: "daemon",
  S5b: "mock",
  S5c: "mock",
  S6a: "daemon",
  S7a: "daemon",
  S7b: "daemon",
  S7c: "daemon",
  S8a: "daemon",
  S8b: "daemon",
  S8c: "daemon",
  S9: "daemon",
  S10: "daemon",
  S11: "daemon",
  S12: "mock",
  S13: "daemon",
  S14: "mock",
  S15: "mock",
  S16: "mock",
  S17: "daemon",
  S18: "daemon",
  S19a: "daemon",
  S19b: "daemon",
  S20: "daemon",
  S21: "daemon",
  S22: "daemon",
  S23: "daemon",
  S24a: "daemon",
  S24b: "mock",
  S25: "daemon",
  S26a: "daemon",
  S26b: "daemon",
  S26c: "daemon",
  S27: "daemon",
  S28: "daemon",
  S29a: "daemon",
  S29b: "daemon",
  S29c: "daemon",
  S29d: "daemon",
  S29e: "daemon",
  S29f: "daemon",
  S29g: "daemon",
  S29h: "daemon",
  S30: "daemon",
  S31a: "daemon",
  S31b: "daemon",
  S32: "daemon",
};

/** True when the section reads from the daemon. Pass another table to ask about it in a test. */
export function isDaemon(
  id: SectionId,
  table: Readonly<Record<SectionId, SectionStatus>> = sectionStatus,
): boolean {
  return table[id] === "daemon";
}
