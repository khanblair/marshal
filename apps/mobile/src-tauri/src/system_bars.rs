use serde::Serialize;
use tauri::{
    plugin::{Builder, PluginHandle, TauriPlugin},
    AppHandle, Manager, Runtime,
};

#[cfg(target_os = "android")]
const ANDROID_PLUGIN_ID: &str = "com.marshal.mobile.systembars";

struct SystemBars<R: Runtime>(Option<PluginHandle<R>>);

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct SetSystemBarsArgs {
    dark: bool,
}

/// Registers the Android system bar adapter and exposes one narrowly scoped command.
pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("marshal-mobile")
        .invoke_handler(tauri::generate_handler![set_system_bars])
        .setup(|app, api| {
            #[cfg(target_os = "android")]
            let handle = api.register_android_plugin(ANDROID_PLUGIN_ID, "SystemBarsPlugin")?;

            #[cfg(not(target_os = "android"))]
            let handle = {
                let _ = api;
                None::<PluginHandle<R>>
            };

            #[cfg(target_os = "android")]
            app.manage(SystemBars(Some(handle)));
            #[cfg(not(target_os = "android"))]
            app.manage(SystemBars(handle));
            Ok(())
        })
        .build()
}

#[tauri::command]
async fn set_system_bars<R: Runtime>(app: AppHandle<R>, dark: bool) -> Result<(), String> {
    let state = app.state::<SystemBars<R>>();
    let Some(handle) = state.0.as_ref() else {
        return Ok(());
    };

    handle
        .run_mobile_plugin_async::<()>("setSystemBars", SetSystemBarsArgs { dark })
        .await
        .map_err(|error| error.to_string())
}
