import { createSignal } from "solid-js";

/** Whether a pairing code is on show, kept while the Remote control section is switched away. */
export function createRemoteDraft() {
  const [pairing, setPairing] = createSignal(false);
  return {
    pairing,
    showPairingCode: () => setPairing(true),
  };
}

export type RemoteDraft = ReturnType<typeof createRemoteDraft>;
