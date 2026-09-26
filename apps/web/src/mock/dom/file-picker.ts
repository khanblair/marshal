/** The kinds of image the daemon takes for an avatar. The picker offers only these. */
const IMAGE_TYPES = "image/png,image/jpeg,image/webp";

/**
 * Opens the browser's file chooser for one image and resolves with the file the person picked, or
 * null when they closed it without picking. The input is never put on the page: a chooser opened by
 * a click works from a detached input.
 */
export function pickImage(): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = IMAGE_TYPES;
    input.addEventListener("change", () => resolve(input.files?.[0] ?? null), { once: true });
    input.addEventListener("cancel", () => resolve(null), { once: true });
    input.click();
  });
}
