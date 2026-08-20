package app.reyna.capture

import android.os.FileObserver
import android.util.Log
import java.io.File

/**
 * Notices files the moment WhatsApp finishes writing them.
 *
 * This is the *fast* path, not the reliable one. A FileObserver dies with its
 * process, so it can never be the system of record — [ReconcileScanner] is.
 * Together they make "never misses a file" true: the observer catches things
 * within a second, and the scan catches everything the observer was not alive
 * for.
 *
 * Two details that are easy to get wrong:
 *
 * FileObserver does not recurse, so every directory needs its own instance.
 *
 * Only `CLOSE_WRITE` and `MOVED_TO` are watched. `CREATE` and `MODIFY` fire
 * while a download is still in progress, which would mean hashing a partial
 * file and recording a file that does not exist yet.
 */
class MediaWatcher(
    private val onFileSettled: (File) -> Unit,
) {
    private val observers = mutableListOf<FileObserver>()

    fun start() {
        stop()
        val dirs = WhatsAppPaths.watchedDirectories()
        if (dirs.isEmpty()) {
            Log.i(TAG, "no WhatsApp media directories visible; nothing to watch")
            return
        }
        for (dir in dirs) {
            val obs = object : FileObserver(dir, CLOSE_WRITE or MOVED_TO) {
                override fun onEvent(event: Int, path: String?) {
                    val name = path ?: return
                    val file = File(dir, name)
                    if (!WhatsAppPaths.isInteresting(file)) return
                    onFileSettled(file)
                }
            }
            obs.startWatching()
            observers += obs
            Log.i(TAG, "watching ${dir.absolutePath}")
        }
    }

    fun stop() {
        observers.forEach { it.stopWatching() }
        observers.clear()
    }

    companion object {
        private const val TAG = "ReynaWatcher"

        /**
         * Waits for a file to stop changing before anyone reads it.
         *
         * `CLOSE_WRITE` is usually enough, but WhatsApp does not always emit it
         * cleanly, and a file arriving by `MOVED_TO` may still be settling.
         * Reading too early yields a truncated file, and since Reyna dedups by
         * content hash, a partial read would be stored as a distinct file and
         * then never reconciled with the real one.
         *
         * Returns false if the file never settles, in which case the next
         * reconcile scan picks it up.
         */
        fun awaitSettled(
            file: File,
            attempts: Int = 5,
            quietMillis: Long = 1_500,
            sleep: (Long) -> Unit = { Thread.sleep(it) },
        ): Boolean {
            var lastSize = -1L
            var lastMtime = -1L
            repeat(attempts) {
                if (!file.isFile) return false
                val size = file.length()
                val mtime = file.lastModified()
                if (size > 0 && size == lastSize && mtime == lastMtime) return true
                lastSize = size
                lastMtime = mtime
                sleep(quietMillis)
            }
            return false
        }
    }
}
