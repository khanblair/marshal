const WEAK_MODEL = /haiku|mini|flash|deepseek|qwen/;
/** Only the roles that judge other agents' code get the warning. */
const CHECKING_ROLES = ["Reviewer", "Integrator"];

/** The warning for a lighter model on a checking role, or null when there is nothing to warn about. */
export function weakModelWarning(model: string, roleName: string): string | null {
  if (!WEAK_MODEL.test(model) || !CHECKING_ROLES.includes(roleName)) return null;
  const when = roleName === "Reviewer" ? "review" : "merge";
  return `${model} is a lighter model. Weak models can break code at ${when} time. You can still save it.`;
}
