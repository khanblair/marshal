/*
 * Card keys. Each project numbers its own cards, so a number alone is not unique: a card is known
 * by its project and its number, written "<projectId>#<number>" (for example "api#41"). This is the
 * format the daemon uses (`protocol.CardKey` in daemon/internal/protocol/ids.go), and the rules of
 * `parseCardKey` mirror the daemon's `ParseCardKey`. The visible label of a card stays "#41".
 * Nothing here depends on the mock's seed.
 */

/** A card's key in the form `<projectId>#<number>`, such as `api#41`. It is the same everywhere. */
export type CardKey = string;

interface ParsedCardKey {
  projectId: string;
  number: number;
}

const SEPARATOR = "#";
/** The daemon reads the number with 31 bits, so 2^31 - 1 is the largest number. */
const MAX_CARD_NUMBER = 2_147_483_647;
/** A project id: lower case letters, digits, and hyphens, 2 to 24 characters, starting with a letter. */
const PROJECT_ID = /^[a-z][a-z0-9-]{1,23}$/;
/** A whole number of 1 or more, with no sign, no decimals, and no leading zero. */
const CARD_NUMBER = /^[1-9][0-9]*$/;

/** Builds the key of card `number` in a project. It does not check its inputs. */
export const cardKey = (projectId: string, number: number): CardKey =>
  `${projectId}${SEPARATOR}${number}`;

/**
 * Reads a key written by `cardKey`. It is strict, like the daemon: the project id must be valid
 * (so a key with an empty project id is refused) and the number must be a whole number of 1 or
 * more. The last hash sign separates the number.
 */
export function parseCardKey(key: string): ParsedCardKey | null {
  const at = key.lastIndexOf(SEPARATOR);
  if (at < 0) return null;
  const projectId = key.slice(0, at);
  const digits = key.slice(at + SEPARATOR.length);
  if (!PROJECT_ID.test(projectId) || !CARD_NUMBER.test(digits)) return null;
  const number = Number(digits);
  return number <= MAX_CARD_NUMBER ? { projectId, number } : null;
}

/** The number in a key, which is what the label `#41` shows. It is 0 when the text is not a card key. */
export const cardNumber = (key: CardKey): number => parseCardKey(key)?.number ?? 0;

/** The visible label of a card: its number in its project, such as `#41`. */
export const cardLabel = (card: { n: number }): string => `${SEPARATOR}${card.n}`;

/**
 * The label for a list that can show cards of more than one project: the project name, one space,
 * then the number, such as `api-gateway #41`. A component that can lay out two elements shows the
 * name and `cardLabel(card)` apart instead of joining them into one string.
 */
export const cardLabelIn = (card: { n: number }, projectName: string): string =>
  `${projectName} ${cardLabel(card)}`;

/** The number a new card in a project gets: one more than the highest number there, or 1. */
export function nextCardNumber(
  cards: Iterable<{ p: string; n: number }>,
  projectId: string,
): number {
  let highest = 0;
  for (const c of cards) if (c.p === projectId && c.n > highest) highest = c.n;
  return highest + 1;
}
