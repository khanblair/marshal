import { createSignal } from "solid-js";

/**
 * Which row of a list has its inline form open, and the error that form shows. The design keeps
 * one for the provider keys and another for the schedules, so both can be open at once.
 */
export function createEditState() {
  const [id, setId] = createSignal<string | null>(null);
  const [error, setError] = createSignal<string | null>(null);
  return {
    id,
    /** The error text, only while the row it belongs to is the open one. */
    errorFor: (rowId: string): string | null => (id() === rowId ? error() : null),
    open(rowId: string): void {
      setId(rowId);
      setError(null);
    },
    close(): void {
      setId(null);
      setError(null);
    },
    /** Opens the row, or closes it when it is already open. */
    toggle(rowId: string): void {
      setId(id() === rowId ? null : rowId);
      setError(null);
    },
    /** Opens another row and keeps the error, as the design's New schedule does. */
    moveTo(rowId: string): void {
      setId(rowId);
    },
    fail(message: string): void {
      setError(message);
    },
  };
}

export type EditState = ReturnType<typeof createEditState>;
