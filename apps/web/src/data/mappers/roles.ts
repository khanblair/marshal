import type { CreateRoleRequest, RoleSpec, Role as WireRole } from "@marshal/protocol";

/**
 * One role as the screens hold it: the wire's `Role` with its spec lifted onto the row, so the role
 * editor reads `role.model` rather than `role.spec.model`. It is the shape
 * `apps/web/src/mock/settings-types.ts` has always described, moved here so the mapper never depends
 * on the mock; `mock/settings-types.ts` re-exports it for the store's own use.
 *
 * The name is the role's address: a card, a chat target, and every route name a role by it.
 */
export interface Role {
  name: string;
  /** True for a role Marshal shipped. A starter role is reset rather than deleted. */
  starter: boolean;
  /** True when the project this list was read for keeps its own version of the role. */
  overridden: boolean;
  skills: string[];
  mcp: string[];
  limits: { time: number; cost: number; rounds: number };
  backup: string;
  desc: string;
  agent: string;
  model: string;
  think: string;
  perm: string;
  strength: string;
  instr: string;
}

/** Every role route's list answer, as the store holds it. */
export interface RoleRows {
  roles: Role[];
}

/** One wire role as the store holds it. */
export function toRole(role: WireRole): Role {
  return {
    name: role.name,
    starter: role.starter,
    overridden: role.overridden,
    skills: [...role.spec.skills],
    mcp: [...role.spec.mcp],
    limits: { ...role.spec.limits },
    backup: role.spec.backup,
    desc: role.spec.desc,
    agent: role.spec.agent,
    model: role.spec.model,
    think: role.spec.think,
    perm: role.spec.perm,
    strength: role.spec.strength,
    instr: role.spec.instr,
  };
}

/**
 * The whole list answer, in the order the daemon sent it. A change route answers the same shape, so
 * the same function applies a read, a save, and a remove.
 */
export function toRoleRows(answer: { roles: readonly WireRole[] }): RoleRows {
  return { roles: answer.roles.map(toRole) };
}

/** A role's editable body as the wire wants it, for a save, an override, or an export. */
export function toRoleSpec(role: Role): RoleSpec {
  return {
    skills: [...role.skills],
    mcp: [...role.mcp],
    limits: { time: role.limits.time, cost: role.limits.cost, rounds: role.limits.rounds },
    backup: role.backup,
    desc: role.desc,
    agent: role.agent,
    model: role.model,
    think: role.think,
    perm: role.perm,
    strength: role.strength,
    instr: role.instr,
  };
}

/** One role as the export document: the name and the body, which is exactly what an import reads. */
export function toExport(role: Role): CreateRoleRequest {
  return { name: role.name, spec: toRoleSpec(role) };
}

/** How much of a role's body the import reads, and how many roles one import may carry. */
const MAX_IMPORT = 200;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const asText = (value: unknown): string => (typeof value === "string" ? value : "");

const asTextList = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];

const asCount = (value: unknown): number =>
  typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : 0;

/** One role document's body, with every field defaulted so a partial export still imports. */
function readSpec(value: unknown): RoleSpec {
  const spec = isRecord(value) ? value : {};
  const limits = isRecord(spec.limits) ? spec.limits : {};
  return {
    skills: asTextList(spec.skills),
    mcp: asTextList(spec.mcp),
    limits: {
      time: asCount(limits.time),
      cost: asCount(limits.cost),
      rounds: asCount(limits.rounds),
    },
    backup: asText(spec.backup),
    desc: asText(spec.desc),
    agent: asText(spec.agent),
    model: asText(spec.model),
    think: asText(spec.think),
    perm: asText(spec.perm),
    strength: asText(spec.strength),
    instr: asText(spec.instr),
  };
}

/**
 * Reads a pasted export: one role document, or an array of them. The id, the starter flag, and the
 * overridden flag are the daemon's to set, so they are ignored here exactly as the route ignores
 * them. It throws a plain sentence for text that is not an export, which the dialog shows as it is.
 */
export function readRoleExports(text: string): CreateRoleRequest[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    throw new Error("That is not JSON. Paste what Export gave you, or a role's own export.");
  }
  const docs = Array.isArray(parsed) ? parsed : [parsed];
  if (docs.length === 0) throw new Error("That export has no roles in it.");
  if (docs.length > MAX_IMPORT) {
    throw new Error(`That export has more than ${MAX_IMPORT} roles in it. Import them in parts.`);
  }
  return docs.map((doc, at) => {
    if (!isRecord(doc)) throw new Error(`Entry ${at + 1} is not a role.`);
    const name = asText(doc.name).trim();
    if (!name) throw new Error(`Entry ${at + 1} has no name, and a role needs one.`);
    return { name, spec: readSpec(doc.spec) };
  });
}

/** Every role's document, as the export writes them, in the order the store holds them. */
export function toExports(roles: readonly Role[]): CreateRoleRequest[] {
  return roles.map(toExport);
}
