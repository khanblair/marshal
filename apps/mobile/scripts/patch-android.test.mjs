import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { patchGradle, patchMainActivity, patchManifest } from "./patch-android.mjs";

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
