/** The most characters a name may have. The daemon holds the same limit. */
export const MAX_NAME_CHARS = 100;
/** The longest email address there is (RFC 5321). The daemon holds the same limit. */
export const MAX_EMAIL_CHARS = 254;

const MIN_NAME_CHARS = 2;
/** One address only: no spaces, no `<name>` form, one `@`, and a dotted domain. */
const EMAIL_SHAPE = /^[^\s@<>()[\],;:"\\]+@(?:[\p{L}\p{N}-]+\.)+\p{L}{2,}$/u;

/** What is wrong with a name, in a sentence, or an empty string when it is fine. */
export function nameProblem(value: string): string {
  const name = value.trim();
  if (!name) return "Enter your name. It shows on cards you comment on.";
  if ([...name].length < MIN_NAME_CHARS) return "Enter your full name, at least 2 characters.";
  if ([...name].length > MAX_NAME_CHARS)
    return `A name can have at most ${MAX_NAME_CHARS} characters.`;
  if (!/\p{L}/u.test(name)) return "A name needs at least one letter.";
  return "";
}

/** What is wrong with an email, in a sentence. An email is required to set up the account. */
export function emailProblem(value: string): string {
  const email = value.trim();
  if (!email) return "Enter your email address. Briefs and alerts are sent there.";
  if (email.length > MAX_EMAIL_CHARS || !EMAIL_SHAPE.test(email)) {
    return "That does not look like an email address. Use the form name@example.com.";
  }
  return "";
}
