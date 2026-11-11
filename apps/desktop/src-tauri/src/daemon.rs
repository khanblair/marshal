//! Checks whether the daemon is up, and starts it (as the per-user service described in
//! docs/architecture.md section on installation) when it is not. The shell never talks to the
//! daemon's API itself: it only checks health here, then hands the window over to the daemon's
//! own address (internal/webui in the daemon serves the real app from there).

use std::time::Duration;

use tauri_plugin_shell::ShellExt;

/// The daemon's normal (non-dev) port, `internal/config.DefaultPort`. The desktop app always
/// runs the normal daemon, never the dev one: `--dev` is for a developer running the source
/// tree, which this packaged app never does.
pub const PORT: u16 = 47800;

/// How long to wait for the daemon to answer once installed, before giving up and telling the
/// person instead of leaving them looking at the splash page forever.
const READY_TIMEOUT: Duration = Duration::from_secs(20);
const POLL_INTERVAL: Duration = Duration::from_millis(300);

/// The daemon's own address, once it is up.
pub fn url() -> String {
    format!("http://127.0.0.1:{PORT}")
}

/// True once `GET /v1/health` answers with success.
async fn healthy(expected_version: &str) -> bool {
    let address = format!("{}/v1/health", url());
    match reqwest::Client::new()
        .get(address)
        .timeout(Duration::from_secs(2))
        .send()
        .await
    {
        Ok(resp) if resp.status().is_success() => resp
            .json::<serde_json::Value>()
            .await
            .ok()
            .and_then(|value| {
                value
                    .get("version")
                    .and_then(|v| v.as_str())
                    .map(str::to_owned)
            })
            .is_some_and(|version| version == expected_version),
        _ => false,
    }
}

/// Keep a matching service. Otherwise run the installer from this shell's bundle
/// and wait for the new version before loading the daemon-served page.
pub async fn ensure_running(app: &tauri::AppHandle) -> Result<(), String> {
    let expected = app.package_info().version.to_string();
    if healthy(&expected).await {
        return Ok(());
    }
    let install = app
        .shell()
        .sidecar("marshal")
        .map_err(|e| e.to_string())?
        .args(["service", "install"])
        .output();
    let output = tokio::time::timeout(READY_TIMEOUT, install)
        .await
        .map_err(|_| "Installing the background service took too long.".to_string())?
        .map_err(|e| e.to_string())?;
    if !output.status.success() {
        let detail = String::from_utf8_lossy(&output.stderr);
        return Err(format!(
            "The background service could not be installed. {}",
            detail.trim()
        ));
    }
    let deadline = std::time::Instant::now() + READY_TIMEOUT;
    while std::time::Instant::now() < deadline {
        if healthy(&expected).await {
            return Ok(());
        }
        tokio::time::sleep(POLL_INTERVAL).await;
    }
    Err(format!(
        "The background service did not answer with version {expected}."
    ))
}

/// The owner's own access token, the same one `marshal token --show` prints
/// (cmd/marshal/cmd_token.go): the daemon writes it to a file on install and keeps it there, so
/// the shell reads it the same way a person at a terminal would, rather than knowing the file's
/// path itself. "The desktop app gets one on install" (docs/architecture.md) is this call: with
/// it, the web app never has to show its sign-in screen on this machine.
pub async fn owner_token(app: &tauri::AppHandle) -> Option<String> {
    let output = app
        .shell()
        .sidecar("marshal")
        .ok()?
        .args(["token", "--show"])
        .output()
        .await
        .ok()?;
    if !output.status.success() {
        eprintln!(
            "marshal token --show: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        );
        return None;
    }
    let token = String::from_utf8_lossy(&output.stdout).trim().to_string();
    if token.is_empty() {
        None
    } else {
        Some(token)
    }
}
