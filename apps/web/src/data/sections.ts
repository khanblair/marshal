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
  | "S27"
  | "S28"
  | "S29a"
  | "S29b"
  | "S29c"
  | "S29d"
  | "S29e"
  | "S29f"
  | "S29g"
  | "S30"
  | "S31a"
  | "S31b"
  | "S32";

export type SectionStatus = "mock" | "daemon";

/**
 * Which sections are on the daemon. The register in docs/backend-checklist.md section 2.2 is the
 * one place that says so, and a test fails when this table and that register differ, so a section
 * is switched in the same change that updates the register. S1 (connection and sign-in), S3 (projects), and S4 (agents and models) are on the daemon.
 */
export const sectionStatus: Readonly<Record<SectionId, SectionStatus>> = {
  S1: "daemon",
  S2a: "daemon",
  S2b: "mock",
  S2c: "mock",
  S3: "daemon",
  S4: "daemon",
  S5a: "daemon",
  S5b: "mock",
  S5c: "mock",
  S6a: "daemon",
  S7a: "daemon",
  S7b: "mock",
  S7c: "daemon",
  S8a: "daemon",
  S8b: "mock",
  S8c: "mock",
  S9: "daemon",
  S10: "daemon",
  S11: "daemon",
  S12: "mock",
  S13: "mock",
  S14: "mock",
  S15: "mock",
  S16: "mock",
  S17: "daemon",
  S18: "daemon",
  S19a: "daemon",
  S19b: "mock",
  S20: "daemon",
  S21: "mock",
  S22: "mock",
  S23: "mock",
  S24a: "daemon",
  S24b: "mock",
  S25: "mock",
  S26a: "mock",
  S26b: "mock",
  S27: "mock",
  S28: "mock",
  S29a: "mock",
  S29b: "mock",
  S29c: "mock",
  S29d: "mock",
  S29e: "mock",
  S29f: "mock",
  S29g: "mock",
  S30: "mock",
  S31a: "daemon",
  S31b: "mock",
  S32: "daemon",
};

/** True when the section reads from the daemon. Pass another table to ask about it in a test. */
export function isDaemon(
  id: SectionId,
  table: Readonly<Record<SectionId, SectionStatus>> = sectionStatus,
): boolean {
  return table[id] === "daemon";
}
