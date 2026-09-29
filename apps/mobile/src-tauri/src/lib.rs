//! The Marshal phone app. It is a remote control, not a second Marshal: the daemon and the agents
//! keep running on the person's computer, and this shell opens the page that daemon serves over
//! the tailnet. `splash/index.html` is what shows until an address is known, and it hands the
//! window to the daemon's own page from there (docs/mobile.md sections 1 and 2).

/// Starts the app. Deep links (`marshal://`) are on everywhere; the camera reader and haptics are
/// phone-only plugins, so they are added only when the crate is built for a phone.
#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default().plugin(tauri_plugin_deep_link::init());
    #[cfg(mobile)]
    let builder = builder
        .plugin(tauri_plugin_barcode_scanner::init())
        .plugin(tauri_plugin_haptics::init());
    builder
        .run(tauri::generate_context!())
        .expect("marshal mobile failed to start");
}
