#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const OWNER = "khanblair";
const REPOSITORY = "marshal";
const ARTIFACT = "Marshal.app.tar.gz";
const ENDPOINT = `https://github.com/${OWNER}/${REPOSITORY}/releases/latest/download/latest.json`;
const VERSION_RE =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;

export function parseReleaseTag(tag) {
  if (typeof tag !== "string" || !/^v?\d/.test(tag))
    throw new Error("Release tag must be a version like v1.2.3.");
  const version = tag.startsWith("v") ? tag.slice(1) : tag;
  if (!VERSION_RE.test(version)) throw new Error(`Invalid release version tag: ${tag}`);
  return version;
}

function latestReleaseNotes(tag) {
  let previousTag;
  try {
    previousTag = execFileSync("git", ["describe", "--tags", "--abbrev=0", `${tag}^`], {
      encoding: "utf8",
    }).trim();
  } catch {
    previousTag = undefined;
  }
  const range = previousTag ? `${previousTag}..${tag}` : tag;
  const subjects = execFileSync("git", ["log", "--pretty=format:%s", range], { encoding: "utf8" })
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  return subjects.length
    ? subjects.map((line) => `- ${line}`).join("\n")
    : `Release ${parseReleaseTag(tag)}.`;
}

export function createReleaseConfig({ tag, publicKey }) {
  const version = parseReleaseTag(tag);
  if (typeof publicKey !== "string" || !publicKey.trim())
    throw new Error("MARSHAL_UPDATER_PUBLIC_KEY is required to create updater release artifacts.");
  return {
    version,
    bundle: { createUpdaterArtifacts: true },
    plugins: { updater: { pubkey: publicKey.trim(), endpoints: [ENDPOINT] } },
  };
}

export function createManifest({
  tag,
  artifactPath,
  signature,
  notes,
  pubDate = new Date().toISOString(),
}) {
  const version = parseReleaseTag(tag);
  if (typeof signature !== "string" || !signature.trim())
    throw new Error("Tauri updater signature is missing or empty.");
  if (typeof notes !== "string" || !notes.trim())
    throw new Error("Release notes are missing or empty.");
  if (!Number.isFinite(Date.parse(pubDate)))
    throw new Error("Release date must be a valid ISO date.");
  const url = `https://github.com/${OWNER}/${REPOSITORY}/releases/download/${encodeURIComponent(tag)}/${encodeURIComponent(basename(artifactPath))}`;
  const platform = { signature: signature.trim(), url };
  return {
    version,
    notes: notes.trim(),
    pub_date: new Date(pubDate).toISOString(),
    platforms: { "darwin-aarch64": platform, "darwin-x86_64": { ...platform } },
  };
}

function usage() {
  return "Usage: prepare-release.mjs config <tag> <output.json> | manifest <tag> <bundle-dir> <output.json>";
}

function main(args) {
  const [mode, tag, input, output] = args;
  if (mode === "resolve-tag" && tag && input && !output) {
    const version = parseReleaseTag(tag);
    writeFileSync(input, `name=${tag}\nversion=${version}\n`);
    return;
  }
  if (mode === "set-version" && tag && input && !output) {
    const config = JSON.parse(readFileSync(input, "utf8"));
    config.version = parseReleaseTag(tag);
    writeFileSync(input, `${JSON.stringify(config, null, 2)}\n`);
    return;
  }
  if (mode === "config" && tag && input && !output) {
    const privateKey = process.env.TAURI_SIGNING_PRIVATE_KEY;
    if (!privateKey)
      throw new Error(
        "TAURI_SIGNING_PRIVATE_KEY secret is required; refusing to build an unsigned, non-updatable release.",
      );
    process.env.MARSHAL_RELEASE_VERSION = parseReleaseTag(tag);
    writeFileSync(
      input,
      `${JSON.stringify(createReleaseConfig({ tag, publicKey: process.env.MARSHAL_UPDATER_PUBLIC_KEY }), null, 2)}\n`,
      { mode: 0o600 },
    );
    return;
  }
  if (mode === "manifest" && tag && input && output) {
    const artifactPath = join(input, ARTIFACT);
    if (!existsSync(artifactPath)) throw new Error(`Updater archive is missing: ${artifactPath}`);
    const signature = readFileSync(`${artifactPath}.sig`, "utf8");
    const notes = latestReleaseNotes(tag);
    writeFileSync(
      output,
      `${JSON.stringify(createManifest({ tag, artifactPath, signature, notes }), null, 2)}\n`,
    );
    return;
  }
  throw new Error(usage());
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
