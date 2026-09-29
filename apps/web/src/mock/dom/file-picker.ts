import { platform } from "~/platform";

/** The kinds of image the daemon takes for an avatar. The picker offers only these. */
const IMAGE_TYPES = "image/png,image/jpeg,image/webp";

/**
 * Opens the device's file chooser for one image and resolves with the file the person picked, or
 * null when they closed it without picking.
 */
export async function pickImage(): Promise<File | null> {
  const [file] = await platform().pickFiles({ accept: IMAGE_TYPES });
  return file ?? null;
}
