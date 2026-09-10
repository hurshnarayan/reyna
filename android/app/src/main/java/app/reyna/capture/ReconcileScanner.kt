package app.reyna.capture

import android.util.Log
import java.io.File
import java.security.MessageDigest

/**
 * Walks WhatsApp's media folders and reports everything Reyna has not seen.
 *
 * **This is the source of truth for capture**, not the FileObserver. The
 * observer dies with its process, gets killed by aggressive OEM battery
 * managers, and cannot see anything that arrived before storage access was
 * granted. The scan has none of those failure modes: a file is either on disk
 * or it is not.
 *
 * Run every fifteen minutes, on app open, and after boot. Because it repeats,
 * it must be cheap for the common case where nothing is new — hence the
 * path-and-mtime prefilter before any hashing.
 */
class ReconcileScanner(
    private val scanRoots: () -> List<File> = WhatsAppPaths::scanRoots,
    /** Whether Reyna has already recorded this exact path and modification time. */
    private val isKnown: (path: String, mtime: Long) -> Boolean,
) {

    data class Found(
        val file: File,
        val sha256: String,
        val sizeBytes: Long,
        val mtimeMillis: Long,
        /** Under a `/Sent/` folder: the user shared it, attributable for free. */
        val isSent: Boolean,
    )

    /**
     * Returns files not yet recorded.
     *
     * Hashing is deliberately the last step. Path plus mtime rules out the
     * overwhelming majority on a repeat scan, so a device with two thousand
     * files does not read two thousand files every quarter hour.
     */
    /**
     * How recently a file must have changed for it to be worth waiting on.
     *
     * Generous, because the cost of waiting on one file that did not need it is
     * a third of a second and the cost of reading one that was mid-write is a
     * truncated document stored under its own hash forever.
     */
    private val RECENT_MILLIS = 60_000L

    fun scan(onProgress: (Int) -> Unit = {}): List<Found> {
        val out = ArrayList<Found>()
        for (dir in scanRoots()) {
            for (f in dir.walkTopDown().onFail { file, error ->
                Log.w(TAG, "cannot scan ${file.name}: ${error.message}")
            }) {
                if (!f.isFile || !WhatsAppPaths.isInteresting(f)) continue
                val mtime = f.lastModified()
                if (isKnown(f.absolutePath, mtime)) continue
                // Only wait on files that could plausibly still be open.
                //
                // awaitSettled cannot return true until its second reading, so
                // it sleeps at least once per file it is asked about. Applied
                // to every file, a first scan of a phone holding a few hundred
                // WhatsApp documents spent minutes asleep confirming that
                // files written months ago were not being written to, and the
                // onboarding screen sat on "Looking through your files" the
                // whole time. A file untouched for a minute is not mid-write.
                if (System.currentTimeMillis() - mtime < RECENT_MILLIS &&
                    !MediaWatcher.awaitSettled(f, attempts = 2, quietMillis = 300)
                ) continue
                val hash = try {
                    sha256(f)
                } catch (e: Exception) {
                    Log.w(TAG, "cannot hash ${f.name}: ${e.message}")
                    continue
                }
                out += Found(
                    file = f,
                    sha256 = hash,
                    sizeBytes = f.length(),
                    mtimeMillis = mtime,
                    isSent = WhatsAppPaths.isSent(f),
                )
                // Reported as they are found so the first run can show a number
                // climbing rather than a spinner that gives no sign of life.
                onProgress(out.size)
            }
        }
        return out
    }

    companion object {
        private const val TAG = "ReynaScan"

        /**
         * Content hash, streamed rather than loaded whole: study groups share
         * hundred-megabyte scans and a 2GB phone will not tolerate reading one
         * into memory.
         *
         * Dedup keys on this because the observer and the scan will both see
         * the same file, and because the same document forwarded twice is one
         * document.
         */
        fun sha256(file: File): String {
            val digest = MessageDigest.getInstance("SHA-256")
            file.inputStream().use { input ->
                val buf = ByteArray(64 * 1024)
                while (true) {
                    val n = input.read(buf)
                    if (n <= 0) break
                    digest.update(buf, 0, n)
                }
            }
            return digest.digest().joinToString("") { "%02x".format(it) }
        }
    }
}
