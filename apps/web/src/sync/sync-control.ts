/** What the store keeps of its link with the daemon, so an action can ask for the snapshots again. */
export interface SyncControl {
  /** Loads every snapshot again and applies it. It never rejects: a failure is shown on `S.loadError`. */
  reload(): Promise<void>;
  /** Stops following the daemon. */
  stop(): void;
}
