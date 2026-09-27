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
            let token = daemon::owner_token(&app).await;
            if let Some(window) = app.get_webview_window("main") {
                if let Ok(url) = daemon::url().parse() {
                    let _ = window.navigate(url);
                }
                // "The desktop app gets one on install" (docs/architecture.md): sign the window
                // in with the owner's own token, the same one `marshal token --show` prints, so
                // the person never sees the plain-browser sign-in screen on this machine. The
                // navigation above only starts loading the new page; a few short retries give it
                // time to reach a document `eval` can run against before giving up quietly (a
                // failure here just leaves the sign-in screen showing, not a crash).
                if let Some(token) = token {
                    seed_token(&window, &token).await;
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

/// Writes the token into the page's own `localStorage` (data/token.ts's `TOKEN_KEY`) and reloads,
/// so the app boots already signed in. `eval` runs once, after a short wait for `navigate` to
/// finish loading the new page: `eval` reports only whether the call reached the webview, not
/// which document it ran against, so retrying on its result cannot tell a too-early attempt from
/// a real failure, and retrying blindly risks firing again after the reload this already caused
/// and reloading the signed-in app a second time. The script's own origin check is the real
/// guard: run early against the splash page by mistake, it does nothing, and the person falls
/// back to the sign-in screen's always-working manual flow instead of a wrong write.
#[cfg(not(debug_assertions))]
async fn seed_token(window: &tauri::WebviewWindow, token: &str) {
    let Ok(token_js) = serde_json::to_string(token) else { return };
    tokio::time::sleep(std::time::Duration::from_millis(500)).await;
    let script = format!(
        "if (location.origin === {origin:?}) {{ \
           localStorage.setItem('marshal-token', {token_js}); location.reload(); \
         }}",
        origin = daemon::url(),
    );
    let _ = window.eval(&script);
}
