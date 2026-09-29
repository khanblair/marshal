// Installs the Rust targets a phone build needs and checks the Android tools (docs/mobile.md
// section 9). It changes nothing outside rustup's own targets and tells the person what is missing.

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";

const TARGETS = [
  "aarch64-linux-android",
  "armv7-linux-androideabi",
  "i686-linux-android",
  "x86_64-linux-android",
];

function need(name, value, hint) {
  const ok = Boolean(value) && existsSync(value);
  console.log(`${ok ? "ok     " : "MISSING"} ${name}${ok ? "" : `: ${hint}`}`);
  return ok;
}

const added = spawnSync("rustup", ["target", "add", ...TARGETS], { stdio: "inherit" });
if (added.status !== 0) {
  console.error("Could not add the Rust Android targets. Is rustup installed?");
  process.exit(1);
}
const sdk = process.env.ANDROID_HOME ?? process.env.ANDROID_SDK_ROOT;
const results = [
  need("Android SDK", sdk, "install Android Studio and set ANDROID_HOME"),
  need("Android NDK", process.env.NDK_HOME, "install the NDK in Android Studio and set NDK_HOME"),
  need("JDK 17", process.env.JAVA_HOME, "install JDK 17 and set JAVA_HOME"),
];
process.exit(results.every(Boolean) ? 0 : 1);
