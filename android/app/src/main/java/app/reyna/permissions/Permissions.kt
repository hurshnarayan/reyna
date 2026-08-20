package app.reyna.permissions

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.os.PowerManager
import android.provider.Settings
import androidx.core.app.NotificationManagerCompat

/**
 * The three permissions Reyna needs, what breaks without each, and how to ask.
 *
 * None of these can be requested with the normal runtime dialog. All three are
 * Settings screens the user has to be walked to, which is why the app has to be
 * explicit about why rather than firing a system prompt and hoping.
 */
object Permissions {

    enum class Kind { Notifications, Storage, Battery }

    data class State(val kind: Kind, val granted: Boolean) {
        val title: String get() = when (kind) {
            Kind.Notifications -> "Notification access"
            Kind.Storage -> "All files access"
            Kind.Battery -> "Battery exemption"
        }

        /**
         * What the user loses, not a restatement of the name. A toggle labelled
         * "Notification access" tells nobody why to grant it.
         */
        val consequence: String get() = when (kind) {
            Kind.Notifications ->
                if (granted) "Reyna can see who shared a file"
                else "Without this, Reyna cannot tell who shared a file"
            Kind.Storage ->
                if (granted) "Reyna can read WhatsApp's media folder"
                else "Without this, Reyna cannot see any files at all"
            Kind.Battery ->
                if (granted) "Reyna keeps watching in the background"
                else "Without this, Reyna stops while you sleep"
        }

        /** Storage is the only one Reyna cannot function at all without. */
        val required: Boolean get() = kind == Kind.Storage
    }

    /**
     * Whether Reyna can read WhatsApp's media folder.
     *
     * On API 30+ this is MANAGE_EXTERNAL_STORAGE, which has no runtime prompt.
     * Below that the legacy read permission covers it.
     */
    fun hasStorage(context: Context): Boolean =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            Environment.isExternalStorageManager()
        } else {
            context.checkSelfPermission(android.Manifest.permission.READ_EXTERNAL_STORAGE) ==
                android.content.pm.PackageManager.PERMISSION_GRANTED
        }

    /**
     * Whether the notification listener is enabled.
     *
     * Checked against the system's own list rather than a local flag, because
     * the user can revoke it in Settings at any time and the app would otherwise
     * keep claiming it is watching.
     */
    fun hasNotificationAccess(context: Context): Boolean =
        NotificationManagerCompat.getEnabledListenerPackages(context).contains(context.packageName)

    fun hasBatteryExemption(context: Context): Boolean {
        val pm = context.getSystemService(Context.POWER_SERVICE) as? PowerManager ?: return false
        return pm.isIgnoringBatteryOptimizations(context.packageName)
    }

    fun all(context: Context): List<State> = listOf(
        State(Kind.Notifications, hasNotificationAccess(context)),
        State(Kind.Storage, hasStorage(context)),
        State(Kind.Battery, hasBatteryExemption(context)),
    )

    /**
     * Opens the Settings screen for a permission.
     *
     * Every one of these is a jump out of the app, so the caller has to re-check
     * state on resume rather than assuming the trip succeeded.
     */
    fun request(activity: Activity, kind: Kind) {
        val intent = when (kind) {
            Kind.Notifications -> Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS)

            Kind.Storage ->
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    // Deep-links straight to Reyna's own entry rather than the
                    // full list, which on some devices is hundreds of apps long.
                    Intent(
                        Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION,
                        Uri.parse("package:${activity.packageName}"),
                    )
                } else {
                    Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS).apply {
                        data = Uri.parse("package:${activity.packageName}")
                    }
                }

            Kind.Battery -> Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
                data = Uri.parse("package:${activity.packageName}")
            }
        }
        try {
            activity.startActivity(intent)
        } catch (_: Exception) {
            // Some OEM builds do not ship the deep-linked screen. Falling back
            // to app details is worse but reachable, and a dead button is worse
            // than either.
            runCatching {
                activity.startActivity(
                    Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS).apply {
                        data = Uri.parse("package:${activity.packageName}")
                    }
                )
            }
        }
    }
}
