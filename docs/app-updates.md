# App updates

The installed macOS desktop app checks for a newer signed release when its UI opens and every four hours while it stays open. A check never downloads or installs an update. Metadata checks time out after 30 seconds; an explicitly requested archive download has a separate 15-minute deadline so slow connections can finish. An available version produces a dismissible notice and an indicator beside **Updates** near the bottom of the sidebar. **Settings → Updates** shows the installed version, available version, release notes, download progress, and errors. On phones, the same Settings page is reachable through the existing navigation.

**Download and install** is an explicit action. After installation, **Restart Marshal** replaces the background service with the updated bundled daemon, waits for the expected daemon version to answer, then restarts the desktop shell. Finish running work before restarting. If restarting fails, the page keeps the restart action and displays the error. A normal launch keeps a daemon running when its version matches the shell; a mismatched version is replaced from the installed app.

A browser cannot install a native desktop application. In the browser or mobile client, this page explains where updates are installed and offers the release list. The phone application itself continues to use its existing distribution channel; this feature does not add Android APK installation.

## Signed release setup

Updater signing was configured with owner approval on 2026-11-05. GitHub stores the private signing key as an Actions secret and its matching public key as a repository variable. A private backup is kept outside the repository. Required configuration names:

- GitHub Actions secret `TAURI_SIGNING_PRIVATE_KEY`.
- Optional GitHub Actions secret `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`.
- GitHub Actions variable `MARSHAL_UPDATER_PUBLIC_KEY`, containing the matching public key.

Generate the key with the Tauri CLI signer and keep a secure backup outside the repository. Never commit or print the private key. Keep the same key for future releases so existing installations can verify them. These signing keys are separate from Apple Developer ID signing and notarization.

The release workflow refuses to publish the desktop build without update-signing configuration. It validates the release tag, sets the shell and both Go sidecars to the release version, builds the universal macOS app, and publishes its `.app.tar.gz`, detached signature, and `latest.json` alongside the DMG. Both `darwin-aarch64` and `darwin-x86_64` entries point to the universal archive. The manifest includes the version, release date, and commit-based release notes.

The fixed feed is `https://github.com/khanblair/marshal/releases/latest/download/latest.json`. The native Tauri updater verifies the archive signature before installation. JavaScript cannot supply an endpoint, signing key, executable, or download path. Only the main installed window at the daemon's fixed loopback origin receives the four scoped update commands; Rust also checks that origin. Generic updater or command-execution permissions are not granted to the daemon page.

Source builds have no signing configuration and explicitly show that updates are disabled. Existing builds shipped before this feature need one manual installation of an update-enabled signed release; they cannot acquire a missing updater through their old UI.

See the [Tauri updater documentation](https://v2.tauri.app/plugin/updater/) for signing-key backup and artifact details.

## Verification

- Frontend tests exercise startup checking, version/notes rendering, explicit installation, deferred prompts, duplicate clicks, errors, browser behavior, restart recovery, and sidebar navigation.
- Rust tests cover the accepted origin/window and serialization states; a native release build verifies the packaged command registration and permissions.
- `node --test apps/desktop/scripts/prepare-release.test.mjs` checks release configuration and manifest generation.
- A real signed upgrade between two installed releases remains a release acceptance check. It must use the production signing configuration and is not replaced by these fixture tests.

Release runs accept a `target` choice (`all`, `desktop`, or `android`) when started manually. This lets a failed platform be retried for the same source tag without rebuilding the other platform. Tag pushes still build both. Android SDK setup uses the supported action that no longer requests Google’s removed `tools` package.
