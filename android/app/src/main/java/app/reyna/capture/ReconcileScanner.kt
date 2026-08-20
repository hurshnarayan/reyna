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
    fun scan(): List<Found> {
        val out = ArrayList<Found>()
        for (dir in WhatsAppPaths.watchedDirectories()) {
            val entries = dir.listFiles() ?: continue
            for (f in entries) {
                if (!f.isFile || !WhatsAppPaths.isInteresting(f)) continue
                val mtime = f.lastModified()
                if (isKnown(f.absolutePath, mtime)) continue
                // Skip anything still being written; the next scan will catch it.
                if (!MediaWatcher.awaitSettled(f, attempts = 2, quietMillis = 300)) continue
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
