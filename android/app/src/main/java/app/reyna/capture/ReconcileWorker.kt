package app.reyna.capture

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import app.reyna.data.Repo
import app.reyna.permissions.Permissions
import java.util.concurrent.TimeUnit

/**
 * The scan that guarantees nothing is missed.
 *
 * The FileObserver is fast but fragile: it dies with its process, is killed by
 * aggressive OEM battery managers, and cannot see anything that arrived before
 * storage access was granted. A scan has none of those failure modes, because
 * a file is either on disk or it is not. This is the source of truth for
 * capture; the observer is only an optimisation on top of it.
 */
class ReconcileWorker(
    context: Context,
    params: WorkerParameters,
) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result {
        val repo = Repo.get(applicationContext)
        if (!repo.capturing) return Result.success()
        if (!Permissions.hasStorage(applicationContext)) return Result.success()

        return runCatching {
            repo.reconcile()
            // Attribution can improve without a new file: a notification seen
            // since the last run may explain something already captured.
            repo.reattributeWeak()
            repo.syncPending()
            Result.success()
        }.getOrElse {
            // Retried with backoff rather than dropped: a transient failure
            // must not mean a file is never seen.
            Result.retry()
        }
    }

    companion object {
        private const val NAME = "reyna_reconcile"

        /**
         * Fifteen minutes is WorkManager's floor for periodic work. Not
         * throttled by battery or network state, because the scan is local and
         * cheap: path plus mtime rules out everything already known before
         * anything is read.
         */
        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<ReconcileWorker>(15, TimeUnit.MINUTES)
                .setConstraints(Constraints.Builder().build())
                .build()
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                NAME,
                // KEEP rather than REPLACE: replacing on every app open resets
                // the interval, so a user who opens the app often would never
                // let a periodic run fire.
                ExistingPeriodicWorkPolicy.KEEP,
                request,
            )
        }

        fun cancel(context: Context) {
            WorkManager.getInstance(context).cancelUniqueWork(NAME)
        }
    }
}

/** Brings capture back after a reboot, without waiting for the app to be opened. */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent?) {
        if (intent?.action != Intent.ACTION_BOOT_COMPLETED) return
        val repo = Repo.get(context)
        if (!repo.capturing || !repo.onboarded) return
        if (!Permissions.hasStorage(context)) return
        CaptureService.start(context)
        ReconcileWorker.schedule(context)
    }
}
