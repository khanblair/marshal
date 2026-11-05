//! Checks whether the daemon is up, and starts it (as the per-user service described in
//! docs/architecture.md section on installation) when it is not. The shell never talks to the
//! daemon's API itself: it only checks health here, then hands the window over to the daemon's
//! own address (internal/webui in the daemon serves the real app from there).

use std::time::Duration;

use tauri_plugin_shell::process::CommandEvent;
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

/// Runs the bundled `marshal` sidecar's `service install`, the same command
/// `marshal service install` runs from a terminal (cmd/marshal/cmd_service.go): it installs the
/// per-user launchd agent for `marshald` if it is not there yet, and starts it either way. Errors
/// are logged, not surfaced as a hard failure here: the health poll after this call is what
/// actually decides whether the app can proceed, since an install racing an already-starting
/// daemon is expected, not exceptional.
fn install_service(app: &tauri::AppHandle) {
    let shell = app.shell();
    let Ok((mut rx, _child)) = shell
        .sidecar("marshal")
        .and_then(|c| c.args(["service", "install"]).spawn())
    else {
        eprintln!("marshal service install: could not start the sidecar");
        return;
    };
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            if let CommandEvent::Stderr(line) = event {
                eprintln!(
                    "marshal service install: {}",
                    String::from_utf8_lossy(&line)
                );
            }
        }
    });
}

/// Ensures the daemon is reachable, installing and starting its service if it was not already
/// running. Returns once it answers, or once `READY_TIMEOUT` passes without an answer.
pub async fn ensure_running(app: tauri::AppHandle) -> bool {
    if healthy(&app.package_info().version.to_string()).await {
        return true;
    }
    install_service(&app);
    let deadline = std::time::Instant::now() + READY_TIMEOUT;
    while std::time::Instant::now() < deadline {
        if healthy(&app.package_info().version.to_string()).await {
            return true;
        }
        tokio::time::sleep(POLL_INTERVAL).await;
    }
    false
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

/// Reinstall the updated bundled service before restarting the shell, so its web UI and daemon
/// advance together. Only the explicit Restart action calls this; normal launches keep sessions.
pub async fn replace_service(app: &tauri::AppHandle, expected_version: &str) -> Result<(), String> {
    let output = app
        .shell()
        .sidecar("marshal")
        .map_err(|e| e.to_string())?
        .args(["service", "install"])
        .output()
        .await
        .map_err(|e| e.to_string())?;
    if !output.status.success() {
        return Err("The updated daemon could not restart. Try Restart again.".into());
    }
    let deadline = std::time::Instant::now() + READY_TIMEOUT;
    while std::time::Instant::now() < deadline {
        if healthy(expected_version).await {
            return Ok(());
        }
        tokio::time::sleep(POLL_INTERVAL).await;
    }
    Err("The updated daemon did not answer. Try Restart again.".into())
}
