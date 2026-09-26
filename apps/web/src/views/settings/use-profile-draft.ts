import { batch, createMemo, createSignal } from "solid-js";
import { M } from "~/mock";

export interface ProfileFields {
  name: string;
  email: string;
  tz: string;
}

const MAX_INITIALS = 2;

const sameFields = (a: ProfileFields, b: ProfileFields): boolean =>
  a.name === b.name && a.email === b.email && a.tz === b.tz;

/** The profile form's edits and the pairing code, kept while the section is switched away. */
export function createProfileDraft() {
  const [edited, setEdited] = createSignal<ProfileFields | null>(null);
  const [pairing, setPairing] = createSignal(false);
  const saved = createMemo<ProfileFields>(() => ({
    name: M.S.profile.name,
    email: M.S.profile.email,
    tz: M.S.profile.tz,
  }));
  const fields = () => edited() ?? saved();
  /** Save is off while nothing changed, and while the name is empty. */
  const cannotSave = () => sameFields(fields(), saved()) || !fields().name.trim();
  return {
    fields,
    cannotSave,
    pairing,
    showPairingCode: () => setPairing(true),
    /**
     * Up to two initials of the name being typed; the saved name when it is empty. Until something
     * is typed they are the daemon's own, when it made them.
     */
    initials: (): string => {
      if (!edited() && M.S.profile.initials) return M.S.profile.initials;
      return (fields().name || saved().name)
        .split(/\s+/)
        .map((word) => word[0] || "")
        .join("")
        .slice(0, MAX_INITIALS)
        .toUpperCase();
    },
    edit(key: keyof ProfileFields, value: string): void {
      setEdited({ ...fields(), [key]: value });
    },
    save(): void {
      if (cannotSave()) return;
      const next = fields();
      // What was typed since the save began is kept: only a form that still holds what was sent is cleared.
      const clear = (): void => {
        if (edited() && sameFields(fields(), next)) setEdited(null);
      };
      batch(() => {
        // The mock saves at once. The daemon answers later, and refuses in its own words, so the form
        // keeps what was typed until it says yes.
        const saved = M.saveProfile({ name: next.name, email: next.email, tz: next.tz });
        if (typeof saved === "boolean") {
          if (saved) clear();
        } else {
          void saved.then((ok) => {
            if (ok) clear();
          });
        }
      });
    },
  };
}

export type ProfileDraft = ReturnType<typeof createProfileDraft>;
