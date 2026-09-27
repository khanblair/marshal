// Prevents an extra console window on Windows in release, in addition to the one Cargo already
// silences with the same attribute below; see https://v2.tauri.app/start/prerequisites/.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod daemon;
mod deeplink;
mod notifications;
mod updater;

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .setup(|app| {
            deeplink::register(app.handle());
            let handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                on_ready(handle).await;
            });
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("marshal desktop failed to start");
}

/// The one thing every launch does once the window exists: in a debug build, `devUrl` in
/// tauri.conf.json already put the vite dev server's own page (with its own daemon, proxied) in
/// the window, so there is nothing to hand off. In a release build, the window is still showing
/// splash/index.html, and this makes sure the real daemon is up before sending it there.
async fn on_ready(app: tauri::AppHandle) {
    #[cfg(debug_assertions)]
    {
        let _ = app;
        return;
    }
    #[cfg(not(debug_assertions))]
    {
        use tauri::Manager;
        if daemon::ensure_running(app.clone()).await {
            if let Some(window) = app.get_webview_window("main") {
                if let Ok(url) = daemon::url().parse() {
                    let _ = window.navigate(url);
                }
            }
            updater::check(&app).await;
        } else {
            notifications::notify(
                &app,
                "Marshal could not start",
                "The daemon did not answer in time. Try opening Marshal again, or run \
                 `marshal service status` from a terminal to see what it says.",
            );
        }
    }
}
