/**
 * The labels of a project in the fake daemon, and the routes that make, rename, recolor, and remove
 * them. A change is announced as `label.updated` on the project's topic, carrying the project's
 * labels as they now are, the way the daemon announces it.
 */
import type { Label, LabelColor } from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { type CardStore, publishCard, type Route, STATUS, topic } from "./fake-card-shared";

/** The labels of a card, by the ids the update request names. */
export function labelsOf(store: CardStore, ids: readonly string[]): Label[] {
  return store.labels.filter((label) => ids.includes(label.id));
}

function newLabel(store: CardStore, projectId: string, name: string, color: LabelColor): Label {
  const label: Label = {
    id: `01M3LABEL000000000000${String(store.labels.length + 1).padStart(2, "0")}`,
    projectId,
    name,
    color,
    createdAt: store.now(),
  };
  store.labels.push(label);
  return label;
}

/** The project's labels as they now are, which is what `label.updated` carries. */
function publishLabels(store: CardStore, projectId: string): void {
  store.publish(topic(projectId), "label.updated", {
    projectId,
    labels: store.labels.filter((label) => label.projectId === projectId),
  });
}

function parseLabels(request: FakeRequest): { name: string; color: LabelColor } {
  const body = JSON.parse(request.body ?? "{}") as { name?: string; color?: LabelColor };
  return { name: (body.name ?? "").trim(), color: body.color ?? "slate" };
}

export function createLabel(store: CardStore, projectId: string, request: FakeRequest): Response {
  const { name, color } = parseLabels(request);
  if (!name) return errorAnswer(STATUS.badRequest, "invalid_argument", "Give the label a name.");
  if (store.labels.some((label) => label.projectId === projectId && label.name === name)) {
    return errorAnswer(STATUS.conflict, "conflict", "That label already exists in this project.");
  }
  const label = newLabel(store, projectId, name, color);
  publishLabels(store, projectId);
  return jsonAnswer(label, STATUS.created);
}

function updateLabel(store: CardStore, label: Label, request: FakeRequest): Response {
  const body = JSON.parse(request.body ?? "{}") as { name?: string; color?: LabelColor };
  if (body.name !== undefined) {
    const name = body.name.trim();
    if (!name) return errorAnswer(STATUS.badRequest, "invalid_argument", "Give the label a name.");
    if (
      store.labels.some(
        (other) =>
          other.id !== label.id && other.projectId === label.projectId && other.name === name,
      )
    ) {
      return errorAnswer(STATUS.conflict, "conflict", "That label already exists in this project.");
    }
    label.name = name;
  }
  if (body.color !== undefined) label.color = body.color;
  publishLabels(store, label.projectId);
  return jsonAnswer(label);
}

function removeLabel(store: CardStore, label: Label): Response {
  const at = store.labels.findIndex((other) => other.id === label.id);
  store.labels.splice(at, 1);
  for (const card of store.cards) {
    if (card.labels.some((onCard) => onCard.id === label.id)) {
      card.labels = card.labels.filter((onCard) => onCard.id !== label.id);
      publishCard(store, card, "card.updated");
    }
  }
  publishLabels(store, label.projectId);
  return emptyAnswer();
}

/** A route under one label: rename or recolor it, or remove it. */
export function labelRoute({ store, request, method }: Route, id: string): Response | undefined {
  const label = store.labels.find((one) => one.id === id);
  if (!label) {
    return errorAnswer(STATUS.notFound, "not_found", "Marshal cannot find that label.");
  }
  if (method === "PATCH") return updateLabel(store, label, request);
  if (method === "DELETE") return removeLabel(store, label);
  return undefined;
}
