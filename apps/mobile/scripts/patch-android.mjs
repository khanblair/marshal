// Makes the Android project Tauri generates fit this app, since `src-tauri/gen/` is generated and
// never committed. Run with --init to make the project first when it does not exist yet.
//
// Six changes:
//  1. Allow plain http. The daemon is reached at a tailnet address such as
//     http://marshal-laptop.tail1234.ts.net:47800, and Android refuses cleartext traffic in a
//     release build by default. The traffic never leaves the person's own tailnet, where WireGuard
//     already encrypts it.
//  2. Take "Share to Marshal" from other apps. Text shared to the app is rewritten, before the deep
//     link plugin looks at it, into a marshal://share link, so one path opens both.
//  3. Sign a release build with the key in `gen/android/keystore.properties`, when that file exists.
//     CI writes it from repository secrets (.github/workflows/release.yml); without it the build is
//     left unsigned, which is what a person building on their own machine wants.
//  4. Reach the Tauri CLI from Gradle. Gradle runs `pnpm tauri ...` inside src-tauri, which is not a
//     pnpm package, and pnpm 11 answers "Command tauri not found" from there. It is run through the
//     workspace's own filter instead, which finds the CLI from any folder.
//  6. Optionally leave the release Java and Kotlin code unshrunk, for finding a crash that only the
//     shrunk build has: MARSHAL_ANDROID_MINIFY=0. The default is shrunk, which is smaller.
//  5. Use Marshal's own launcher icons. `tauri android init` always makes the project with Tauri's
//     default icon and never reads src-tauri/icons/android, so the icons committed there were
//     never applied; they are copied over the generated ones, adaptive icon and all.

import { spawnSync } from "node:child_process";
import { cpSync, existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const SHARE_FILTER = `            <intent-filter>
                <action android:name="android.intent.action.SEND" />
                <category android:name="android.intent.category.DEFAULT" />
                <data android:mimeType="text/plain" />
            </intent-filter>
`;

const MAIN_ACTIVITY = `package com.marshal.mobile

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import androidx.activity.enableEdgeToEdge

class MainActivity : TauriActivity() {
  override fun onCreate(savedInstanceState: Bundle?) {
    enableEdgeToEdge()
    shareToLink(intent)
    super.onCreate(savedInstanceState)
  }

  override fun onNewIntent(intent: Intent) {
    shareToLink(intent)
    super.onNewIntent(intent)
  }

  // Text shared from another app becomes a marshal://share link, which the deep link plugin hands to
  // the page like any other link.
  private fun shareToLink(intent: Intent?) {
    if (intent?.action != Intent.ACTION_SEND || intent.type?.startsWith("text/") != true) return
    val text = intent.getStringExtra(Intent.EXTRA_TEXT) ?: return
    val link = Uri.Builder().scheme("marshal").authority("share").appendQueryParameter("text", text)
    intent.getStringExtra(Intent.EXTRA_SUBJECT)?.let { link.appendQueryParameter("title", it) }
    intent.action = Intent.ACTION_VIEW
    intent.data = link.build()
  }
}
`;

/** Adds the "share text to this app" filter to the main activity, once. */
export function patchManifest(xml) {
  if (xml.includes("android.intent.action.SEND")) return xml;
  return xml.replace(/(\s*)<\/activity>/, `\n${SHARE_FILTER.trimEnd()}$1</activity>`);
}

const SIGNING = `    val keystoreFile = rootProject.file("keystore.properties")
    val keystore = Properties().apply {
        if (keystoreFile.exists()) keystoreFile.inputStream().use { load(it) }
    }
    signingConfigs {
        create("release") {
            keyAlias = keystore["keyAlias"] as String?
            keyPassword = keystore["keyPassword"] as String?
            storeFile = (keystore["storeFile"] as String?)?.let { file(it) }
            storePassword = keystore["storePassword"] as String?
        }
    }
`;

/**
 * Turns cleartext traffic on in every build type, by the placeholder the manifest reads, and signs
 * the release build with the key in keystore.properties when there is one.
 */
export function patchGradle(kts, { minify = true } = {}) {
  let patched = kts.replace(/(optimization\s*\{\s*enable\s*=\s*)(?:true|false)/, `$1${minify}`);
  patched = patched.replace(
    /manifestPlaceholders\["usesCleartextTraffic"\] = "false"/,
    'manifestPlaceholders["usesCleartextTraffic"] = "true"',
  );
  if (patched.includes("keystore.properties")) return patched;
  patched = patched.replace(/(\n\s*buildTypes \{)/, `\n${SIGNING.trimEnd()}$1`);
  return patched.replace(
    /(getByName\("release"\) \{)/,
    '$1\n            if (keystoreFile.exists()) signingConfig = signingConfigs.getByName("release")',
  );
}

/** The activity that turns a text share into a link. Anything else is left as it is. */
export function patchMainActivity(kotlin) {
  if (kotlin.includes("shareToLink")) return kotlin;
  return kotlin.includes("class MainActivity : TauriActivity()") ? MAIN_ACTIVITY : kotlin;
}

/** Runs the Tauri CLI through pnpm's workspace filter, so it is found from src-tauri. */
export function patchBuildTask(kotlin) {
  return kotlin.replace(
    'listOf("tauri", "android", "android-studio-script")',
    'listOf("--filter", "mobile", "exec", "tauri", "android", "android-studio-script")',
  );
}

/**
 * Copies the app's own launcher icons (src-tauri/icons/android) over the generated project's, folder
 * by folder: the PNGs of every density, the adaptive icon, and the color it sits on.
 */
export function copyLauncherIcons(iconsDir, resDir) {
  if (!existsSync(iconsDir)) {
    console.error(`Missing ${iconsDir}. The launcher icons are committed there.`);
    process.exit(1);
  }
  cpSync(iconsDir, resDir, { recursive: true, force: true });
}

function patchFile(path, patch) {
  if (!existsSync(path)) {
    console.error(
      `Missing ${path}. The Android template changed; update scripts/patch-android.mjs.`,
    );
    process.exit(1);
  }
  writeFileSync(path, patch(readFileSync(path, "utf8")));
}

function main() {
  const root = join(dirname(fileURLToPath(import.meta.url)), "..");
  const project = join(root, "src-tauri", "gen", "android");
  if (process.argv.includes("--init") && !existsSync(project)) {
    const init = spawnSync("pnpm", ["exec", "tauri", "android", "init", "--ci"], {
      cwd: root,
      stdio: "inherit",
    });
    if (init.status !== 0) process.exit(init.status ?? 1);
  }
  const app = join(project, "app");
  patchFile(join(app, "src", "main", "AndroidManifest.xml"), patchManifest);
  const minify = process.env.MARSHAL_ANDROID_MINIFY !== "0";
  patchFile(join(app, "build.gradle.kts"), (kts) => patchGradle(kts, { minify }));
  patchFile(
    join(app, "src", "main", "java", "com", "marshal", "mobile", "MainActivity.kt"),
    patchMainActivity,
  );
  patchFile(
    join(
      project,
      "buildSrc",
      "src",
      "main",
      "java",
      "com",
      "marshal",
      "mobile",
      "kotlin",
      "BuildTask.kt",
    ),
    patchBuildTask,
  );
  copyLauncherIcons(join(root, "src-tauri", "icons", "android"), join(app, "src", "main", "res"));
  console.log(
    "The Android project allows the tailnet's plain-http address, takes shared text, and has Marshal's icons.",
  );
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
