//! OS notifications shown while the app is open (docs/mobile.md's desktop-side equivalent of
//! "local notices"). v1 covers only what the shell itself needs to tell the person about: the
//! daemon failing to start. The daemon's own notice kinds (B5.6, B9.4) arrive over its event
//! stream once the web app is showing, and are the web app's job to render, not this shell's.

use tauri::AppHandle;
use tauri_plugin_notification::NotificationExt;

pub fn notify(app: &AppHandle, title: &str, body: &str) {
    if let Err(err) = app.notification().builder().title(title).body(body).show() {
        // A notification the OS refused (permission not granted, or not supported) is not worth
        // failing the app over: the window's own splash text already says the same thing.
        eprintln!("notification failed: {err}");
    }
}
