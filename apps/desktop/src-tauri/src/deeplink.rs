//! `marshal://` links (docs/mobile.md: "Open marshal:// links... from notices, Telegram,
//! Discord, and ntfy"). v1 handles the link the same way on every platform this shell runs on:
//! bring the window forward and hand its path and query to the daemon's own address, where the
//! web app's own router (already built, screen by screen) is what actually opens the right card
//! or approval. This file only does the OS-level plumbing of receiving the link.

use tauri::{AppHandle, Manager};
use tauri_plugin_deep_link::DeepLinkExt;

use crate::daemon;

/// Registers the handler for every `marshal://` URL the OS hands the app, at any point after
/// launch (a cold start's own first link is queued by the plugin and delivered as if it arrived
/// this way too, so one handler covers both cases).
pub fn register(app: &AppHandle) {
    let app = app.clone();
    app.clone().deep_link().on_open_url(move |event| {
        for target in event.urls() {
            open(&app, target.as_str());
        }
    });
}

/// Turns `marshal://card/abc123` into the daemon's own address for the same path and brings the
/// window forward to show it. Silently does nothing for a URL that is not this app's own scheme,
/// which should not reach here, but a malformed or foreign link is not worth a crash over.
fn open(app: &AppHandle, target: &str) {
    let Some(rest) = target.strip_prefix("marshal://") else {
        return;
    };
    let Some(window) = app.get_webview_window("main") else {
        return;
    };
    let destination = format!("{}/{}", daemon::url(), rest);
    if let Ok(url) = destination.parse() {
        let _ = window.navigate(url);
    }
    let _ = window.show();
    let _ = window.set_focus();
}
