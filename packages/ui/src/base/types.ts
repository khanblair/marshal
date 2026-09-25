/**
 * Any string, while editors still suggest the known literals it is joined with.
 * Used for values that come from data, such as `IconName | LooseString`.
 */
export type LooseString = string & Record<never, never>;
