//! A fixed, signed update feed. Checking never installs; installation requires an explicit action.
use serde::Serialize;
use std::sync::Mutex;
use tauri::{AppHandle, State, WebviewWindow};
use tauri_plugin_updater::{Update, UpdaterExt};

#[derive(Clone, Default, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Snapshot {
    status: String,
    current_version: String,
    version: Option<String>,
    notes: Option<String>,
    downloaded: u64,
    total: Option<u64>,
    error: Option<String>,
}
#[derive(Default)]
struct Pending {
    snapshot: Snapshot,
    update: Option<Update>,
}
#[derive(Default)]
pub struct UpdateState(Mutex<Pending>);

// The local daemon serves this window. No URL, command, or signing key comes from JavaScript.
fn authorize(window: &WebviewWindow) -> Result<(), String> {
    let url = window.url().map_err(|e| e.to_string())?;
    if trusted_origin(window.label(), &url.origin().ascii_serialization()) {
        Ok(())
    } else {
        Err("Updates are available only in the installed Marshal desktop app.".into())
    }
}
fn trusted_origin(label: &str, origin: &str) -> bool {
    label == "main" && origin == crate::daemon::url()
}

fn configured(app: &AppHandle) -> bool {
    app.config().plugins.0.get("updater").is_some_and(|v| {
        v.get("pubkey")
            .and_then(|v| v.as_str())
            .is_some_and(|v| !v.is_empty())
            && v.get("endpoints")
                .and_then(|v| v.as_array())
                .is_some_and(|v| !v.is_empty())
    })
}
fn busy(status: &str) -> bool {
    matches!(
        status,
        "checking" | "downloading" | "installing" | "installed" | "restarting"
    )
}
#[tauri::command]
pub fn update_status(
    app: AppHandle,
    window: WebviewWindow,
    state: State<UpdateState>,
) -> Result<Snapshot, String> {
    authorize(&window)?;
    let mut pending = state.0.lock().map_err(|e| e.to_string())?;
    if pending.snapshot.status.is_empty() {
        pending.snapshot.status = if configured(&app) { "idle" } else { "disabled" }.into();
        pending.snapshot.current_version = app.package_info().version.to_string();
    }
    Ok(pending.snapshot.clone())
}
#[tauri::command]
pub async fn check_update(
    app: AppHandle,
    window: WebviewWindow,
    state: State<'_, UpdateState>,
) -> Result<Snapshot, String> {
    authorize(&window)?;
    if !configured(&app) {
        return update_status(app, window, state);
    }
    {
        let mut pending = state.0.lock().map_err(|e| e.to_string())?;
        if busy(&pending.snapshot.status) {
            return Ok(pending.snapshot.clone());
        }
        pending.update = None;
        pending.snapshot = Snapshot {
            status: "checking".into(),
            current_version: app.package_info().version.to_string(),
            ..Default::default()
        };
    }
    let result = match app
        .updater_builder()
        .timeout(std::time::Duration::from_secs(30))
        .build()
    {
        Ok(updater) => updater.check().await.map_err(|e| e.to_string()),
        Err(err) => Err(err.to_string()),
    };
    let mut pending = state.0.lock().map_err(|e| e.to_string())?;
    match result {
        Ok(Some(update)) => {
            pending.snapshot.status = "available".into();
            pending.snapshot.version = Some(update.version.clone());
            pending.snapshot.notes = update.body.clone();
            pending.update = Some(update);
        }
        Ok(None) => pending.snapshot.status = "current".into(),
        Err(err) => {
            pending.snapshot.status = "error".into();
            pending.snapshot.error = Some(err);
        }
    }
    Ok(pending.snapshot.clone())
}
#[tauri::command]
pub async fn install_update(
    window: WebviewWindow,
    state: State<'_, UpdateState>,
) -> Result<Snapshot, String> {
    authorize(&window)?;
    let update = {
        let mut pending = state.0.lock().map_err(|e| e.to_string())?;
        if pending.snapshot.status != "available" {
            return Err("Check for an available update first.".into());
        }
        let update = pending.update.take().ok_or("Check for updates again.")?;
        pending.snapshot.status = "downloading".into();
        update
    };
    let result = update
        .download_and_install(
            |bytes, total| {
                if let Ok(mut pending) = state.0.lock() {
                    pending.snapshot.downloaded += bytes as u64;
                    pending.snapshot.total = total;
                }
            },
            || {
                if let Ok(mut pending) = state.0.lock() {
                    pending.snapshot.status = "installing".into();
                }
            },
        )
        .await;
    let mut pending = state.0.lock().map_err(|e| e.to_string())?;
    match result {
        Ok(()) => pending.snapshot.status = "installed".into(),
        Err(err) => {
            pending.snapshot.status = "error".into();
            pending.snapshot.error = Some(err.to_string());
        }
    }
    Ok(pending.snapshot.clone())
}
#[tauri::command]
pub async fn restart_updated_app(
    app: AppHandle,
    window: WebviewWindow,
    state: State<'_, UpdateState>,
) -> Result<(), String> {
    authorize(&window)?;
    let target = {
        let mut pending = state.0.lock().map_err(|e| e.to_string())?;
        if pending.snapshot.status != "installed" {
            return Err("Install the update before restarting.".into());
        }
        pending.snapshot.status = "restarting".into();
        pending
            .snapshot
            .version
            .clone()
            .ok_or("The installed update version is missing.")?
    };
    if let Err(err) = crate::daemon::replace_service(&app, &target).await {
        let mut pending = state.0.lock().map_err(|e| e.to_string())?;
        pending.snapshot.status = "installed".into();
        pending.snapshot.error = Some(err.clone());
        return Err(err);
    }
    app.restart();
}
#[cfg(test)]
mod tests {
    #[test]
    fn only_the_daemon_main_window_may_update() {
        assert!(super::trusted_origin("main", "http://127.0.0.1:47800"));
        for origin in [
            "https://example.com",
            "http://127.0.0.1:47801",
            "http://localhost:47800",
            "http://127.0.0.1:47800.example.com",
        ] {
            assert!(!super::trusted_origin("main", origin));
        }
        assert!(!super::trusted_origin("other", "http://127.0.0.1:47800"));
    }
    #[test]
    fn changes_are_serialized_until_restart() {
        for status in [
            "checking",
            "downloading",
            "installing",
            "installed",
            "restarting",
        ] {
            assert!(super::busy(status));
        }
        assert!(!super::busy("available"));
        assert!(!super::busy("error"));
    }
}
