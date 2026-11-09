fn main() {
    let attributes = tauri_build::Attributes::new().plugin(
        "marshal-mobile",
        tauri_build::InlinedPlugin::new().commands(&["set_system_bars"]),
    );
    tauri_build::try_build(attributes).expect("failed to generate mobile permissions");
}
