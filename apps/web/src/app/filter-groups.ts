/**
 * The Add filter menu's groups and the filter chips' labels, computed from the
 * current project's cards as the design's `renderVals` does.
 */
import { statusTone, toneSolidText } from "@marshal/ui";
import { type Card, type FilterKey, M, type Status } from "~/mock";
import { currentProject } from "./shell-layout";

export const FILTER_KEY_LABEL: Record<FilterKey, string> = {
  status: "Status",
  role: "Role",
  agent: "Agent",
  label: "Label",
  package: "Package",
  model: "Model",
};

interface FilterOption {
  kind: FilterKey;
  /** The value stored in the filter. */
  value: string;
  label: string;
  count: number;
  icon?: string;
  /** Color class of the status icon. */
  iconClass?: string;
}

export interface FilterGroup {
  label: string;
  options: FilterOption[];
}

/** What a chip shows for a filter value: statuses use their label. */
export function chipValue(kind: FilterKey, value: string): string {
  return kind === "status" ? (M.STATUS[value as Status]?.label ?? value) : value;
}

const uniqueSorted = (values: readonly string[]): string[] =>
  Array.from(new Set(values)).filter(Boolean).sort();

type CardTest = (card: Card, value: string) => boolean;

function valueOptions(
  kind: FilterKey,
  values: readonly string[],
  cards: readonly Card[],
  matches: CardTest,
): FilterOption[] {
  return values.map((value) => ({
    kind,
    value,
    label: value,
    count: cards.filter((c) => matches(c, value)).length,
  }));
}

function statusOptions(cards: readonly Card[]): FilterOption[] {
  return M.COLUMNS.map((column) => ({
    kind: "status",
    value: column,
    label: M.STATUS[column].label,
    count: cards.filter((c) => M.colOf(c.state) === column).length,
    icon: M.STATUS[column].icon,
    iconClass: toneSolidText[statusTone(column)],
  }));
}

/** Status, Role, Agent, Label, and (for monorepos) Package options with card counts. */
export function filterGroups(): FilterGroup[] {
  const cards = M.cardsOf(M.S.route.pid ?? "");
  const roles = uniqueSorted(cards.map((c) => c.role));
  const agents = uniqueSorted(cards.map((c) => c.agent));
  const labels = uniqueSorted(cards.flatMap((c) => c.labels));
  const groups: FilterGroup[] = [
    { label: "Status", options: statusOptions(cards) },
    { label: "Role", options: valueOptions("role", roles, cards, (c, v) => c.role === v) },
    { label: "Agent", options: valueOptions("agent", agents, cards, (c, v) => c.agent === v) },
    {
      label: "Label",
      options: valueOptions("label", labels, cards, (c, v) => c.labels.includes(v)),
    },
  ];
  const packages = currentProject().packages;
  if (packages) {
    groups.push({
      label: "Package",
      options: valueOptions("package", packages, cards, (c, v) => c.pkg === v),
    });
  }
  return groups;
}
