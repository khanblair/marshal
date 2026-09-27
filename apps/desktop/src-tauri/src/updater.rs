//! App and daemon updates. v1 only wires the app's own update check through
//! `tauri-plugin-updater`; it is inert until `tauri.conf.json`'s `plugins.updater.active` and
//! `endpoints` point at a real update manifest and `pubkey` holds its signing key, which is not
//! set up yet (release.yml has no signing key to publish one from). The daemon's own update path
//! is a separate, later piece: the daemon updates itself the way `marshal service install`
//! already replaces a running install, not through this plugin.

use tauri::AppHandle;
use tauri_plugin_updater::UpdaterExt;

/// Checks for an app update and installs it if one is found. Called once after the window is
/// showing, not before: an update check should never delay getting the daemon and the UI up.
/// A disabled or unreachable updater is not an error worth telling the person about here.
pub async fn check(app: &AppHandle) {
    let Ok(updater) = app.updater() else {
        return;
    };
    match updater.check().await {
        Ok(Some(update)) => {
            if let Err(err) = update.download_and_install(|_, _| {}, || {}).await {
                eprintln!("update failed: {err}");
            }
        }
        Ok(None) => {}
        Err(err) => eprintln!("update check failed: {err}"),
    }
}
