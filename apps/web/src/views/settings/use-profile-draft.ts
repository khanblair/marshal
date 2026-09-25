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
    /** Up to two initials of the name being typed; the saved name when it is empty. */
    initials: (): string =>
      (fields().name || saved().name)
        .split(/\s+/)
        .map((word) => word[0] || "")
        .join("")
        .slice(0, MAX_INITIALS)
        .toUpperCase(),
    edit(key: keyof ProfileFields, value: string): void {
      setEdited({ ...fields(), [key]: value });
    },
    save(): void {
      if (cannotSave()) return;
      const next = fields();
      batch(() => {
        Object.assign(M.S.profile, {
          name: next.name.trim(),
          email: next.email.trim(),
          tz: next.tz,
        });
        setEdited(null);
        M.toast("Profile saved");
      });
    },
  };
}

export type ProfileDraft = ReturnType<typeof createProfileDraft>;
