import assert from "node:assert/strict";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
  copyLauncherIcons,
  patchBuildTask,
  patchGradle,
  patchMainActivity,
  patchManifest,
} from "./patch-android.mjs";

const MANIFEST = `<manifest>
    <application android:usesCleartextTraffic="\${usesCleartextTraffic}">
        <activity android:name=".MainActivity">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
            </intent-filter>
        </activity>
        <provider android:name="x" />
    </application>
</manifest>`;
const GRADLE = `android {
    defaultConfig {
        manifestPlaceholders["usesCleartextTraffic"] = "false"
    }
    buildTypes {
        getByName("debug") { manifestPlaceholders["usesCleartextTraffic"] = "true" }
        getByName("release") {
            isMinifyEnabled = true
        }
    }
}`;
const ACTIVITY = `class MainActivity : TauriActivity() {\n}`;

test("the main activity takes shared text, inside the activity and once", () => {
  const once = patchManifest(MANIFEST);
  assert.match(
    once,
    /<action android:name="android.intent.action.SEND" \/>[\s\S]*?<\/intent-filter>\s*<\/activity>/,
  );
  assert.match(once, /<data android:mimeType="text\/plain" \/>/);
  assert.equal(patchManifest(once), once);
  assert.equal(once.match(/<\/activity>/g)?.length, 1);
});

test("cleartext is on for every build type, by the placeholder the manifest reads", () => {
  const patched = patchGradle(GRADLE);
  assert.match(
    patched,
    /defaultConfig \{\s*manifestPlaceholders\["usesCleartextTraffic"\] = "true"/,
  );
  assert.doesNotMatch(patched, /"false"/);
});

const SHRINK = `android {
    buildTypes {
        getByName("release") {
            optimization {
               enable = true
            }
        }
    }
}`;

test("the release build is shrunk by default, and can be left unshrunk to find a crash", () => {
  assert.match(patchGradle(SHRINK), /enable = true/);
  const off = patchGradle(SHRINK, { minify: false });
  assert.match(off, /enable = false/);
  assert.match(patchGradle(off, { minify: true }), /enable = true/);
  assert.equal(patchGradle(off, { minify: false }), off);
});

test("a release build is signed with keystore.properties when the file is there, and only then", () => {
  const patched = patchGradle(GRADLE);
  assert.match(patched, /rootProject\.file\("keystore\.properties"\)/);
  assert.match(patched, /signingConfigs \{\s*create\("release"\)/);
  assert.match(
    patched,
    /getByName\("release"\) \{\s*if \(keystoreFile\.exists\(\)\) signingConfig = signingConfigs\.getByName\("release"\)/,
  );
  assert.ok(patched.indexOf("signingConfigs {") < patched.indexOf("buildTypes {"));
  assert.equal(patchGradle(patched), patched);
});

test("the activity turns shared text into a marshal://share link, once", () => {
  const once = patchMainActivity(ACTIVITY);
  assert.match(once, /shareToLink\(intent\)/);
  assert.match(once, /authority\("share"\)/);
  assert.equal(patchMainActivity(once), once);
  assert.equal(patchMainActivity("class Other {}"), "class Other {}");
});

test("the patched activity keeps what the generated one had", () => {
  const generated = `class MainActivity : TauriActivity() {\n  override fun onCreate(savedInstanceState: Bundle?) {\n    enableEdgeToEdge()\n    super.onCreate(savedInstanceState)\n  }\n}`;
  const patched = patchMainActivity(generated);
  assert.match(patched, /enableEdgeToEdge\(\)/);
  assert.match(patched, /super\.onCreate\(savedInstanceState\)/);
});

test("the real generated project, when one exists, takes every patch", {
  skip: !process.env.MARSHAL_ANDROID_GEN,
}, () => {
  const app = "src-tauri/gen/android/app";
  assert.match(
    patchManifest(readFileSync(`${app}/src/main/AndroidManifest.xml`, "utf8")),
    /action.SEND/,
  );
  assert.match(
    patchGradle(readFileSync(`${app}/build.gradle.kts`, "utf8")),
    /"usesCleartextTraffic"\] = "true"/,
  );
});

const ICONS = join(import.meta.dirname, "..", "src-tauri", "icons", "android");
const DENSITIES = ["mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"];

function write(root, path, text) {
  mkdirSync(join(root, path, ".."), { recursive: true });
  writeFileSync(join(root, path), text);
}

test("Marshal's launcher icons replace the ones Tauri generates, and nothing else is touched", () => {
  const source = mkdtempSync(join(tmpdir(), "icons-"));
  const res = mkdtempSync(join(tmpdir(), "res-"));
  write(source, "mipmap-xxxhdpi/ic_launcher.png", "marshal");
  write(source, "mipmap-anydpi-v26/ic_launcher.xml", "adaptive");
  write(source, "values/ic_launcher_background.xml", "white");
  write(res, "mipmap-xxxhdpi/ic_launcher.png", "tauri");
  write(res, "values/strings.xml", "kept");
  copyLauncherIcons(source, res);
  assert.equal(readFileSync(join(res, "mipmap-xxxhdpi/ic_launcher.png"), "utf8"), "marshal");
  assert.equal(readFileSync(join(res, "mipmap-anydpi-v26/ic_launcher.xml"), "utf8"), "adaptive");
  assert.equal(readFileSync(join(res, "values/ic_launcher_background.xml"), "utf8"), "white");
  assert.equal(readFileSync(join(res, "values/strings.xml"), "utf8"), "kept");
  copyLauncherIcons(source, res);
  assert.equal(readFileSync(join(res, "mipmap-xxxhdpi/ic_launcher.png"), "utf8"), "marshal");
});

test("the committed icons are a whole launcher icon: every density, and the adaptive icon with its color", () => {
  for (const density of DENSITIES) {
    for (const name of ["ic_launcher", "ic_launcher_round", "ic_launcher_foreground"]) {
      assert.ok(existsSync(join(ICONS, `mipmap-${density}`, `${name}.png`)), `${density} ${name}`);
    }
  }
  const adaptive = readFileSync(join(ICONS, "mipmap-anydpi-v26", "ic_launcher.xml"), "utf8");
  assert.match(adaptive, /@mipmap\/ic_launcher_foreground/);
  assert.match(adaptive, /@color\/ic_launcher_background/);
  assert.match(
    readFileSync(join(ICONS, "values", "ic_launcher_background.xml"), "utf8"),
    /name="ic_launcher_background"/,
  );
});

test("Gradle reaches the Tauri CLI through pnpm's workspace filter, once", () => {
  const generated = 'val args = listOf("tauri", "android", "android-studio-script");';
  const patched = patchBuildTask(generated);
  assert.match(
    patched,
    /listOf\("--filter", "mobile", "exec", "tauri", "android", "android-studio-script"\)/,
  );
  assert.equal(patchBuildTask(patched), patched);
});
