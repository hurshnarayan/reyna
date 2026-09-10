package app.reyna.capture

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import app.reyna.R
import app.reyna.data.Repo
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * Keeps the file watcher alive while the app is not open.
 *
 * A FileObserver dies with its process, so without a foreground service Reyna
 * would only see files while the user happened to have the app on screen. The
 * service is the fast path; [ReconcileWorker] is the one that guarantees
 * nothing is missed.
 *
 * The persistent notification is not optional on Android and is not a bad
 * thing: an app reading another app's folder and reading notifications should
 * be visibly running rather than invisible.
 */
class CaptureService : Service() {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private lateinit var repo: Repo
    private var watcher: MediaWatcher? = null

    override fun onCreate() {
        super.onCreate()
        repo = Repo.get(this)
        createChannel()
        startForeground(NOTIF_ID, buildNotification("Watching for new files"))

        watcher = MediaWatcher { file ->
            // The observer fires the moment WhatsApp closes the handle, which
            // can still be mid-download, so nothing is read until the file
            // stops changing.
            scope.launch {
                if (!MediaWatcher.awaitSettled(file)) return@launch
                if (!WhatsAppPaths.isInteresting(file)) return@launch
                runCatching {
                    val found = ReconcileScanner.Found(
                        file = file,
                        sha256 = ReconcileScanner.sha256(file),
                        sizeBytes = file.length(),
                        mtimeMillis = file.lastModified(),
                        isSent = WhatsAppPaths.isSent(file),
                    )
                    repo.onFileFound(found)?.let {
                        Log.i(TAG, "captured ${it.name}")
                        repo.syncPending()
                    }
                }.onFailure { Log.w(TAG, "capture failed for ${file.name}: ${it.message}") }
            }
        }.also { it.start() }

        // Anything that arrived while the service was dead.
        scope.launch {
            repo.resetRetiredFiles()
            repo.reconcile()
            repo.reattributeWeak()
            repo.syncPending()
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // Restarted by the system if killed: capture should resume without the
        // user reopening the app.
        return START_STICKY
    }

    override fun onDestroy() {
        watcher?.stop()
        scope.cancel()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun createChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            CHANNEL, "Capture",
            // Low: it must be visible, but it is a status line, not an alert.
            NotificationManager.IMPORTANCE_LOW,
        ).apply { description = "Shows while Reyna is watching for new files" }
        (getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager)
            .createNotificationChannel(channel)
    }

    private fun buildNotification(text: String): Notification {
        val open = PendingIntent.getActivity(
            this, 0,
            packageManager.getLaunchIntentForPackage(packageName),
            PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL)
            .setContentTitle("Reyna")
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_reyna_mark)
            .setOngoing(true)
            .setContentIntent(open)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    companion object {
        private const val TAG = "ReynaCapture"
        private const val CHANNEL = "reyna_capture"
        private const val NOTIF_ID = 1

        fun start(context: Context) {
            val intent = Intent(context, CaptureService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                context.startForegroundService(intent)
            } else {
                context.startService(intent)
            }
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, CaptureService::class.java))
        }
    }
}
