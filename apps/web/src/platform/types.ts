/** Where the app is running: a browser tab, the desktop shell, or the phone app. */
export type PlatformKind = "web" | "desktop" | "mobile";

/** How strong a tap on the phone should feel. */
export type Haptic = "tap" | "success" | "error";

export interface PickOptions {
  /** A comma list of types, as an input's `accept` takes it. */
  accept?: string;
  multiple?: boolean;
  /** Open the camera instead of the file list, where the device has one. */
  camera?: boolean;
}

/**
 * The few things the app needs from the device it runs on. Views call these and never a native
 * feature directly, so each one has a plain fallback on a device that lacks it.
 */
export interface Platform {
  readonly kind: PlatformKind;
  /** True inside the desktop shell or the phone app, false in a browser tab. */
  readonly native: boolean;
  /** The files a person picked, or none when the chooser was closed. */
  pickFiles(options?: PickOptions): Promise<File[]>;
  /** A folder a person picked, or null. Only the desktop shell has a folder picker. */
  pickFolder(): Promise<string | null>;
  /** True where the camera can read a QR code. */
  readonly canScanCode: boolean;
  /** The text of the QR code that was scanned, or null when it was cancelled or cannot be done. */
  scanCode(): Promise<string | null>;
  /** Opens a web address in the person's own browser. False when nothing opened, such as a blocked pop-up. */
  openExternal(url: string): Promise<boolean>;
  /** A short tap. It does nothing where the device has no vibration. */
  haptic(kind: Haptic): void;
  /** Calls the handler with each `marshal://` link that opens the app. Returns a stop function. */
  onDeepLink(handler: (url: string) => void): () => void;
}
