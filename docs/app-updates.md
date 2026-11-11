# App updates

## First install from Terminal

On macOS, use the repository’s [install.sh](../install.sh) for a fresh install. From the repository root, review the script and run `sh ./install.sh`. It downloads the latest public universal DMG, compares its SHA-256 with GitHub’s release asset digest, verifies the copied app’s code signature, and places Marshal in `/Applications`. To choose another writable folder, set `MARSHAL_INSTALL_DIR` to its absolute path.

The Terminal download avoids the quarantine flag applied by browsers. The installer leaves macOS security settings alone. The app still uses ad-hoc Apple signing until Developer ID credentials are configured. The checksum checks the downloaded bytes against the GitHub release and relies on GitHub as the source of trust.

The script stops if Marshal is already installed at the destination. For an existing installation, use **Settings → Updates**, which verifies updates with Marshal’s Tauri signing key.

## Update an installed app

The installed macOS desktop app checks for a newer signed release when its UI opens and every four hours while it stays open. A check never downloads or installs an update. Metadata checks time out after 30 seconds; an explicitly requested archive download has a separate 15-minute deadline so slow connections can finish. An available version produces a dismissible notice and an indicator beside **Updates** near the bottom of the sidebar. **Settings → Updates** shows the installed version, available version, release notes, download progress, and errors. On phones, the same Settings page is reachable through the existing navigation.

**Download and install** is an explicit action. After installation, **Restart Marshal** relaunches the desktop shell. The new shell keeps its startup screen open while it starts the matching bundled daemon. If the background service cannot start, that screen shows the error. Finish running work before restarting. A normal launch keeps a running daemon when its version matches the shell.

A browser cannot install a native desktop application. In the browser or mobile client, this page explains where updates are installed and offers the release list. The phone application itself continues to use its existing distribution channel; this feature does not add Android APK installation.

## Signed release setup

Updater signing was configured with owner approval on 2026-11-05. GitHub stores the private signing key as an Actions secret and its matching public key as a repository variable. A private backup is kept outside the repository. Required configuration names:

- GitHub Actions secret `TAURI_SIGNING_PRIVATE_KEY`.
- Optional GitHub Actions secret `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`.
- GitHub Actions variable `MARSHAL_UPDATER_PUBLIC_KEY`, containing the matching public key.

Generate the key with the Tauri CLI signer and keep a secure backup outside the repository. Never commit or print the private key. Keep the same key for future releases so existing installations can verify them. These signing keys are separate from Apple Developer ID signing and notarization. Empty optional Apple credentials are removed before invoking Tauri; without real Apple credentials the configured ad-hoc signature is used.

The release workflow refuses to publish the desktop build without update-signing configuration. It validates the release tag, sets the shell and both Go sidecars to the release version, builds the universal macOS app, and publishes its `.app.tar.gz`, detached signature, and `latest.json` alongside the DMG. Both `darwin-aarch64` and `darwin-x86_64` entries point to the universal archive. The manifest includes the version, release date, and commit-based release notes.

The fixed feed is `https://github.com/khanblair/marshal/releases/latest/download/latest.json`. The native Tauri updater verifies the archive signature before installation. JavaScript cannot supply an endpoint, signing key, executable, or download path. Only the main installed window at the daemon's fixed loopback origin receives the four scoped update commands; Rust also checks that origin. Generic updater or command-execution permissions are not granted to the daemon page.

Source builds have no signing configuration and explicitly show that updates are disabled. Existing builds shipped before this feature need one manual installation of an update-enabled signed release; they cannot acquire a missing updater through their old UI.

See the [Tauri updater documentation](https://v2.tauri.app/plugin/updater/) for signing-key backup and artifact details.

## Verification

[Marshal v0.1.2](https://github.com/khanblair/marshal/releases/tag/v0.1.2) is the first published update-enabled installer. The GitHub release gate started the extracted daemon on Intel. A separate download check verified the public manifest, archive signature, both architectures, and the bundled command versions on Apple silicon. The installer uses ad-hoc Apple signing.

- Frontend tests exercise startup checking, version/notes rendering, explicit installation, deferred prompts, duplicate clicks, errors, browser behavior, restart recovery, and sidebar navigation.
- Rust tests cover the accepted origin/window and serialization states; a native release build verifies the packaged command registration and permissions.
- Before publication, `scripts/check-bundle.mjs` extracts the signed updater archive without extended attributes, verifies app code signatures and versions, checks both Mac architectures, runs both packaged CLIs, and starts an isolated packaged daemon to verify HTTP health and the embedded update UI. It removes only its own process and temporary data.
- `node --test apps/desktop/scripts/prepare-release.test.mjs` checks release configuration and manifest generation.
- A real signed upgrade between two installed releases remains a release acceptance check. It must use the production signing configuration and is not replaced by these fixture tests.

## Release order

1. Set the requested version in `.github/release.json` and push to `main`.
2. CI runs generation, formatting, lint, types, focused release/card/mobile/runtime regressions, and the other light checks. A failure stops the slower jobs.
3. The full client suites and their coverage floors run on Linux. That is the whole release gate, about eight minutes in total.
4. Beside it, `ci-full.yml` runs the macOS daemon and Go tool tests with race instrumentation, daemon coverage, the build and the size and idle-memory budgets, and the Linux browser tests at phone, tablet, and desktop sizes. They do not block release. Check them before you rely on a build.
5. A successful CI completion triggers `release.yml` on GitHub. It accepts only a push to this repository's `main`, uses that exact verified commit, and skips superseded commits or an already published version. No agent or local watcher is needed to advance this chain.
6. The Mac installer/updater files and signed Android APK/AAB build in parallel. Each job uploads verified artifacts. A missing signing configuration fails the selected build.
7. Only after both builds succeed, the publish job uploads all downloads to a draft, checks their names and sizes and the tag's commit, then makes the release public. Failed new uploads leave a draft, rather than a partially published new release.

Failed CI blocks release. A failed platform build blocks publication. A corrected push starts the chain again with the requested version still pending. The CI summary names the blocking stage; GitHub sends its own workflow notifications according to your account settings. This chain advances successful jobs automatically; it does not automatically edit code after failures.

Tag pushes alone no longer trigger release. GitHub creates the version tag at publication, on the verified commit. Never move an existing version tag. Bump the version if a different source commit is needed for an existing tag.

Manual release runs accept an existing `tag` and a `target` (`all`, `desktop`, or `android`). The tag's exact commit must have a successful main push CI run. A platform-only retry is allowed for an existing published release; a new release requires both platforms. The extra macOS browser workflow is manual only. The browser tests run in `ci-full.yml`, which does not block a release. Android SDK setup uses the supported action that no longer requests Google’s removed `tools` package.

## Hardened runtime and secret scanning

The Go helpers include Gitleaks, whose go-re2 dependency uses Wazero to execute generated regular-expression code. Wazero 1.8.2 does not use `MAP_JIT` for these allocations. A signed helper without the needed permission passes static signature verification but macOS kills it before startup. Local checks reproduced that failure; `allow-jit` alone also failed, while the single `com.apple.security.cs.allow-unsigned-executable-memory` entitlement allowed the signed helpers to start.

`src-tauri/entitlements.plist` grants that one exception, with hardened runtime explicitly enabled. Tauri 2.12 applies the same entitlement file to the shell and external binaries; it has no per-helper configuration. No library-validation bypass or application-sandbox exception is added. The release gate tests the actual signed archive so a development-only startup pass is insufficient.

References: [Apple executable-memory entitlement](https://developer.apple.com/documentation/bundleresources/entitlements/com.apple.security.cs.allow-unsigned-executable-memory), [Tauri 2.12 signing implementation](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.0/crates/tauri-bundler/src/bundle/macos/sign.rs).


## Android release downloads

The release workflow builds both an APK for direct installation and an AAB for Google Play. Signed builds are attached to the GitHub release as `Marshal_<version>_universal.apk` and `Marshal_<version>_universal.aab`. The workflow verifies the APK signature with Android's `apksigner` and the AAB signature with Java's `jarsigner` before publishing them. An unsigned build is kept only as an Actions artifact because it cannot be installed on a phone.

Android signing was configured on 2026-11-09. GitHub Actions uses `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, and `ANDROID_KEY_PASSWORD`. A private keystore and credential backup are kept outside this repository. Keep the same signing key for future APK updates, and back it up securely. This signing key is free and separate from Tauri updater signatures and Apple signing.
