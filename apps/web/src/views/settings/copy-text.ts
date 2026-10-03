import { M } from "~/mock";

/** Puts text on the clipboard and says so. A browser without the clipboard says to copy it by hand. */
export function copyText(text: string, done: string): void {
  if (!navigator.clipboard) {
    M.toast("Copy the text by hand instead.");
    return;
  }
  void navigator.clipboard.writeText(text).then(
    () => M.toast(done),
    () => M.toast("Copy the text by hand instead."),
  );
}
