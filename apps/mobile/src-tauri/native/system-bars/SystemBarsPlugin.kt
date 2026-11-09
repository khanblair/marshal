package com.marshal.mobile.systembars

import android.app.Activity
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.Plugin

@InvokeArg
class SetSystemBarsArgs {
  var dark: Boolean = false
}

@TauriPlugin
class SystemBarsPlugin(private val activity: Activity) : Plugin(activity) {
  @Command
  fun setSystemBars(invoke: Invoke) {
    val args = invoke.parseArgs(SetSystemBarsArgs::class.java)
    activity.runOnUiThread {
      try {
        val controller = WindowInsetsControllerCompat(activity.window, activity.window.decorView)
        controller.show(WindowInsetsCompat.Type.statusBars())
        controller.isAppearanceLightStatusBars = !args.dark
        controller.isAppearanceLightNavigationBars = !args.dark
        invoke.resolve()
      } catch (error: Exception) {
        invoke.reject(error.message ?: "Could not update the system bar appearance")
      }
    }
  }
}
