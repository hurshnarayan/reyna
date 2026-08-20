package app.reyna.capture

import android.os.Environment
import java.io.File

/**
 * Where WhatsApp keeps the files Reyna captures.
 *
 * WhatsApp moved its media under `Android/media/` when scoped storage landed,
 * which is the reason on-device capture is possible at all: that tree stays
 * readable to other apps holding broad storage access, unlike `Android/data/`.
 *
 * Nothing here writes. Reyna reads the folder and never modifies it.
 */
object WhatsAppPaths {

    /** Consumer WhatsApp and WhatsApp Business, which many small groups use. */
    private val PACKAGES = listOf("com.whatsapp", "com.whatsapp.w4b")

    /**
     * Media subfolders worth watching. Documents and images are the ones people
     * actually search for later; audio and video are deliberately left out for
     * now, since they are large and Reyna cannot read them usefully.
     */
    private val MEDIA_DIRS = listOf(
        "WhatsApp Documents",
        "WhatsApp Images",
    )

    /**
     * A file under a `/Sent/` folder was shared by the phone's owner. That is
     * one of the few attributions available for free and at full confidence.
     */
    const val SENT = "Sent"

    private fun root(pkg: String): File =
        File(Environment.getExternalStorageDirectory(), "Android/media/$pkg/WhatsApp/Media")

    /**
     * Every directory to watch, including the `/Sent/` variants.
     *
     * FileObserver does not recurse, so each one needs its own observer. Only
     * directories that exist are returned: a device with no WhatsApp Business
     * install should not spawn observers on phantom paths.
     */
    fun watchedDirectories(): List<File> = buildList {
        for (pkg in PACKAGES) {
            val base = root(pkg)
            if (!base.isDirectory) continue
            for (dir in MEDIA_DIRS) {
                val d = File(base, dir)
                if (d.isDirectory) add(d)
                val sent = File(d, SENT)
                if (sent.isDirectory) add(sent)
            }
        }
    }

    /** Whether this path sits under a `/Sent/` folder. */
    fun isSent(file: File): Boolean = file.parentFile?.name == SENT

    /**
     * Whether any WhatsApp media directory is visible.
     *
     * False means either WhatsApp is not installed or storage access has not
     * been granted. The two are indistinguishable from here, so the caller must
     * check the permission before blaming the install.
     */
    fun anyVisible(): Boolean = PACKAGES.any { root(it).isDirectory }

    /**
     * Extensions worth capturing. Reyna is a document search tool, so a
     * screenshot of a meme is noise. Images are included because photographed
     * notes are extremely common in study groups.
     */
    private val INTERESTING = setOf(
        "pdf", "doc", "docx", "ppt", "pptx", "xls", "xlsx",
        "txt", "rtf", "odt", "csv", "epub",
        "jpg", "jpeg", "png", "webp",
    )

    fun isInteresting(file: File): Boolean =
        file.extension.lowercase() in INTERESTING
}
