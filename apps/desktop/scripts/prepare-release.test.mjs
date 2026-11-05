import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { createManifest, createReleaseConfig, parseReleaseTag } from "./prepare-release.mjs";

const scriptPath = fileURLToPath(new URL("./prepare-release.mjs", import.meta.url));

test("release tags become validated bare semantic versions", () => {
  assert.equal(parseReleaseTag("v1.2.3"), "1.2.3");
  assert.equal(parseReleaseTag("1.2.3-rc.1+build"), "1.2.3-rc.1+build");
  for (const tag of ["v1.2", "v01.2.3", "v1.2.3;echo bad", "main"]) {
    assert.throws(() => parseReleaseTag(tag));
  }
});

test("release config turns on Tauri updater artifacts using a supplied public key", () => {
  const config = createReleaseConfig({ tag: "v2.4.0", publicKey: "  public-key  " });
  assert.equal(config.version, "2.4.0");
  assert.equal(config.bundle.createUpdaterArtifacts, true);
  assert.equal(config.plugins.updater.pubkey, "public-key");
  assert.deepEqual(config.plugins.updater.endpoints, [
    "https://github.com/khanblair/marshal/releases/latest/download/latest.json",
  ]);
  assert.throws(
    () => createReleaseConfig({ tag: "v2.4.0", publicKey: "" }),
    /MARSHAL_UPDATER_PUBLIC_KEY/,
  );
});

test("config command writes a fixture config and refuses to run without signing secrets", () => {
  const directory = mkdtempSync(join(tmpdir(), "marshal-release-"));
  try {
    const output = join(directory, "tauri-release.json");
    const missingKey = spawnSync(process.execPath, [scriptPath, "config", "v2.4.0", output], {
      encoding: "utf8",
      env: {
        ...process.env,
        TAURI_SIGNING_PRIVATE_KEY: "",
        MARSHAL_UPDATER_PUBLIC_KEY: "public-key",
      },
    });
    assert.notEqual(missingKey.status, 0);
    assert.match(missingKey.stderr, /TAURI_SIGNING_PRIVATE_KEY secret is required/);

    const generated = spawnSync(process.execPath, [scriptPath, "config", "v2.4.0", output], {
      encoding: "utf8",
      env: {
        ...process.env,
        TAURI_SIGNING_PRIVATE_KEY: "fixture-secret",
        MARSHAL_UPDATER_PUBLIC_KEY: "fixture-public-key",
      },
    });
    assert.equal(generated.status, 0, generated.stderr);
    const config = JSON.parse(readFileSync(output, "utf8"));
    assert.equal(config.version, "2.4.0");
    assert.equal(config.bundle.createUpdaterArtifacts, true);
    assert.equal(config.plugins.updater.pubkey, "fixture-public-key");
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test("manifest maps the signed universal archive to both macOS architectures", () => {
  const manifest = createManifest({
    tag: "v2.4.0",
    artifactPath: "/tmp/Marshal.app.tar.gz",
    signature: " signature contents\n",
    notes: "- Feature A\n- Fix B",
    pubDate: "2026-11-05T12:30:00Z",
  });
  assert.equal(manifest.version, "2.4.0");
  assert.equal(manifest.notes, "- Feature A\n- Fix B");
  assert.equal(manifest.pub_date, "2026-11-05T12:30:00.000Z");
  assert.deepEqual(manifest.platforms["darwin-aarch64"], manifest.platforms["darwin-x86_64"]);
  assert.equal(manifest.platforms["darwin-aarch64"].signature, "signature contents");
  assert.equal(
    manifest.platforms["darwin-aarch64"].url,
    "https://github.com/khanblair/marshal/releases/download/v2.4.0/Marshal.app.tar.gz",
  );
});

test("manifest command refuses to publish when the archive fixture is absent", () => {
  const directory = mkdtempSync(join(tmpdir(), "marshal-release-"));
  try {
    const result = spawnSync(
      process.execPath,
      [scriptPath, "manifest", "v2.4.0", directory, join(directory, "latest.json")],
      { encoding: "utf8" },
    );
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Updater archive is missing/);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
