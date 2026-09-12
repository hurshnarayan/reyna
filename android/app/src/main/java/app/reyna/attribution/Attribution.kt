package app.reyna.attribution

import java.util.Locale
import java.util.concurrent.TimeUnit
import kotlin.math.abs

/**
 * Decides who shared a file and when.
 *
 * On-device this is the hard problem and the whole product. A file in
 * WhatsApp's media folder carries no sender: the name, the chat and the time
 * have to be reconstructed by matching it against messages we learned about
 * separately (a notification we saw, or a line in a chat export). That match
 * can fail, and it can be ambiguous, so every result carries a confidence and
 * the UI is forbidden from naming anyone below the threshold.
 *
 * The rule that matters: **never invent a sender.** "Shared in Sem 5 CS,
 * eleven weeks ago" is a good answer. "Shared by Mohit" when we are guessing
 * is not, because the user cannot tell the difference and will act on it.
 */
object Attribution {

    /**
     * How a sender was determined, ordered by how much it can be trusted.
     * Mirrors the server-side constants in `internal/model`.
     */
    object Method {
        const val EXPORT = "export"             // Matched a chat export line. Authoritative.
        const val SELF_SENT = "self_sent"       // File was in WhatsApp's /Sent/ folder.
        const val USER = "user"                 // The user told us.
        const val NOTIFICATION = "notification" // Matched an observed notification.
        const val DATE_UNIQUE = "date_unique"   // Only one candidate message that day.
        const val TIME_WINDOW = "time_window"   // Nearest message in time. Ambiguous.
        const val TIME_ONLY = "time_only"       // Something in the same ten minutes.
        const val NONE = ""                     // Unattributed.
    }

    /**
     * Confidence floor for stating who shared a file. Below this Reyna
     * describes what it knows (the chat, the date) and stops, and the name is
     * not passed to the language model either — a model shown a name will use
     * it however the prompt hedges.
     *
     * Must stay in step with `model.AttributionMinNamed` on the server.
     */
    const val MIN_NAMED = 0.70

    /** A message we know about, from a notification or an export. */
    data class Event(
        val id: Long,
        val chatKey: String,
        val chatName: String,
        val senderKey: String,
        val senderDisplay: String,
        val postedAtMillis: Long,
        val text: String,
        /** Filename if the event named one. */
        val attachmentName: String = "",
        val hasAttachment: Boolean = attachmentName.isNotEmpty(),
        val source: String = "",
    )

    /** A file we found on disk. */
    data class CapturedFile(
        val id: Long,
        /** Name as it appears on disk, which may be WhatsApp-generated. */
        val diskName: String,
        /** When the file landed on disk. NOT when it was posted. */
        val mtimeMillis: Long,
        /** True when found under a `/Sent/` directory. */
        val isSent: Boolean = false,
    )

    /** One candidate match, with why and how strongly. */
    data class Link(
        val fileId: Long,
        val eventId: Long,
        val method: String,
        val confidence: Double,
    )

    /**
     * The result for one file. [best] is null when nothing matched, which is a
     * legitimate outcome and must be shown as "found on your phone" rather than
     * hidden.
     */
    data class Attributed(
        val fileId: Long,
        val best: Link?,
        /**
         * Every candidate, not only the winner. Kept so a chat export imported
         * later can promote a better match without the earlier reasoning being
         * lost, and so a wrong guess can be explained after the fact.
         */
        val candidates: List<Link>,
    ) {
        val confidence: Double get() = best?.confidence ?: 0.0
        val method: String get() = best?.method ?: Method.NONE
        /** Whether the sender may be named. */
        val canName: Boolean get() = confidence >= MIN_NAMED
    }

    private val SIX_HOURS = TimeUnit.HOURS.toMillis(6)
    private val TWO_HOURS = TimeUnit.HOURS.toMillis(2)
    private val TEN_MINUTES = TimeUnit.MINUTES.toMillis(10)
    private val ONE_DAY = TimeUnit.DAYS.toMillis(1)

    /**
     * WhatsApp-generated media names: `DOC-20260818-WA0007.pdf`,
     * `IMG-20260818-WA0012.jpg`.
     *
     * These carry two signals worth having even when nothing else matches. The
     * date is exact. The counter is sequential, so `WA0007` arrived after
     * `WA0003` that day — which orders files within a day with no other source
     * at all.
     *
     * Whether documents keep their original name or get renamed to this form is
     * the single highest-value unknown in the whole design: it decides whether
     * rule 1 (exact, 0.95) or rule 4 (date-unique, 0.70) is the common path.
     * Needs one fifteen-minute test on a real device.
     */
    private val WA_NAME = Regex("""^[A-Z]{3}-(\d{4})(\d{2})(\d{2})-WA(\d{4})\.""")

    /** Parses the embedded date, or null when the name is not WhatsApp's. */
    fun waNameDate(diskName: String): Triple<Int, Int, Int>? {
        val m = WA_NAME.find(diskName) ?: return null
        val (y, mo, d) = m.destructured.toList().take(3).map { it.toInt() }
        return Triple(y, mo, d)
    }

    /** Parses the intra-day counter, or null. Lets files be ordered within a day. */
    fun waNameCounter(diskName: String): Int? =
        WA_NAME.find(diskName)?.groupValues?.getOrNull(4)?.toIntOrNull()

    private val IMAGE_EXTENSIONS = setOf("jpg", "jpeg", "png", "webp")
    fun isImage(diskName: String): Boolean =
        diskName.substringAfterLast('.', "").lowercase(Locale.ROOT) in IMAGE_EXTENSIONS

    fun isMediaCompatible(diskName: String, attName: String, text: String): Boolean {
        val fIsImage = isImage(diskName)
        if (attName.isNotEmpty()) {
            val attIsImage = isImage(attName)
            if (fIsImage != attIsImage) return false
            val fExt = diskName.substringAfterLast('.', "").lowercase(Locale.ROOT)
            val attExt = attName.substringAfterLast('.', "").lowercase(Locale.ROOT)
            if (!fIsImage && fExt.isNotEmpty() && attExt.isNotEmpty() && fExt != attExt) return false
            return true
        }
        val t = text.trim()
        if (fIsImage) {
            if (t.startsWith("📄") || t.contains(".pdf", ignoreCase = true) || t.contains(".docx", ignoreCase = true) || t.contains(".pptx", ignoreCase = true)) {
                return false
            }
            return true
        }
        if (t.startsWith("📷") || t.contains("photo", ignoreCase = true)) {
            return false
        }
        return true
    }

    /**
     * Matches one file against the events we know about.
     *
     * The chain runs strongest-first and collects every candidate rather than
     * stopping at the first hit, because a later export can promote a weaker
     * link and we want the alternatives still on record when it does.
     *
     * Runs in both directions in practice: on a new file against stored events,
     * and on a new event against still-unattributed files. That matters because
     * a user can download a file hours or days after it was posted, so neither
     * side can assume it arrives second.
     */
    fun attribute(
        file: CapturedFile,
        events: List<Event>,
        /**
         * Zone used to compare a WhatsApp filename's embedded date against an
         * event's absolute timestamp.
         *
         * WhatsApp stamps `DOC-20260818-WAnnnn` with the phone's *local* date,
         * so local is correct — but it has to be explicit. Left implicit, this
         * silently mis-dates every file near midnight for anyone whose zone
         * differs from wherever the comparison happens to run, and the symptom
         * is a lost attribution rather than an error.
         */
        zone: java.util.TimeZone = java.util.TimeZone.getDefault(),
    ): Attributed {
        val candidates = ArrayList<Link>()

        // Rule 0. The user shared it themselves. Certain, and free.
        if (file.isSent) {
            val link = Link(file.id, eventId = 0, method = Method.SELF_SENT, confidence = 1.0)
            return Attributed(file.id, link, listOf(link))
        }

        val diskLower = file.diskName.lowercase(Locale.ROOT)
        val fileDate = waNameDate(file.diskName)

        for (e in events) {
            if (e.chatName.equals("WhatsApp", ignoreCase = true) || e.senderDisplay.equals("WhatsApp", ignoreCase = true)) continue
            if (e.chatName.isBlank() && e.senderDisplay.isBlank()) continue

            val dt = abs(e.postedAtMillis - file.mtimeMillis)

            // Rules 1-2. The event names this exact file. Nothing beats it.
            if (e.attachmentName.isNotEmpty() &&
                e.attachmentName.lowercase(Locale.ROOT) == diskLower
            ) {
                candidates += if (dt <= SIX_HOURS) {
                    Link(file.id, e.id, Method.EXPORT, 0.95)
                } else {
                    Link(file.id, e.id, Method.EXPORT, 0.85)
                }
                continue
            }

            // Rule 3. The filename appears inside the message text.
            if (diskLower.isNotEmpty() &&
                e.text.lowercase(Locale.ROOT).contains(diskLower) &&
                dt <= TWO_HOURS
            ) {
                candidates += Link(file.id, e.id, Method.NOTIFICATION, 0.75)
                continue
            }

            // Rule 3b. High-confidence temporal notification match for unnamed media attachments (e.g. photos)
            // When an attachment notification arrives within 5 minutes of the file being written,
            // and did not name an unrelated file, it directly explains the new media.
            if (isImage(file.diskName) && e.hasAttachment && e.attachmentName.isEmpty() && dt <= 5 * 60 * 1000L) {
                candidates += Link(file.id, e.id, Method.NOTIFICATION, 0.85)
                continue
            }

            // Rules 4-5. The name is WhatsApp-generated, so it tells us the
            // date but not which message. Resolved below, once we know how many
            // events share that day.
            if (fileDate != null && e.hasAttachment && isMediaCompatible(file.diskName, e.attachmentName, e.text) && sameDay(e.postedAtMillis, fileDate, zone)) {
                candidates += Link(file.id, e.id, Method.DATE_UNIQUE, 0.70)
                continue
            }

            // Rule 6. Nothing but proximity. Weak by construction, and only
            // ever enough to say "shared in this chat", never a name.
            if (e.hasAttachment && isMediaCompatible(file.diskName, e.attachmentName, e.text) && dt <= TEN_MINUTES) {
                candidates += Link(file.id, e.id, Method.TIME_ONLY, 0.30)
            }
        }

        // A date-unique claim is only worth 0.70 when the day really is
        // unique. Several candidates on the same day means we are picking one,
        // which is a guess, so it drops below the naming threshold and the
        // nearest in time wins.
        val dateHits = candidates.filter { it.method == Method.DATE_UNIQUE }
        val resolved = if (dateHits.size > 1) {
            val nearest = dateHits.minByOrNull { link ->
                val e = events.first { it.id == link.eventId }
                abs(e.postedAtMillis - file.mtimeMillis)
            }
            candidates.map {
                when {
                    it.method != Method.DATE_UNIQUE -> it
                    it === nearest -> it.copy(method = Method.TIME_WINDOW, confidence = 0.45)
                    else -> it.copy(method = Method.TIME_WINDOW, confidence = 0.30)
                }
            }
        } else {
            candidates
        }

        val best = resolved.maxWithOrNull(
            compareBy<Link> { it.confidence }
                .thenBy { link ->
                    val ev = events.firstOrNull { it.id == link.eventId }
                    if (!ev?.senderDisplay.isNullOrBlank()) 1 else 0
                }
                .thenByDescending { link ->
                    val ev = events.firstOrNull { it.id == link.eventId }
                    if (ev != null) -abs(ev.postedAtMillis - file.mtimeMillis) else Long.MIN_VALUE
                }
        )
        return Attributed(file.id, best, resolved)
    }

    private fun sameDay(
        millis: Long,
        date: Triple<Int, Int, Int>,
        zone: java.util.TimeZone,
    ): Boolean {
        val cal = java.util.GregorianCalendar(zone).apply { timeInMillis = millis }
        return cal.get(java.util.Calendar.YEAR) == date.first &&
            cal.get(java.util.Calendar.MONTH) + 1 == date.second &&
            cal.get(java.util.Calendar.DAY_OF_MONTH) == date.third
    }

    /**
     * How to describe a file, given what we actually know.
     *
     * This is where honest degradation is enforced. Three bands, and the name
     * only survives the top one.
     */
    fun describe(
        confidence: Double,
        senderName: String?,
        chatName: String?,
        whenText: String,
    ): String = when {
        confidence >= MIN_NAMED && !senderName.isNullOrBlank() ->
            "$senderName · $whenText"
        confidence >= 0.30 && !chatName.isNullOrBlank() ->
            "$chatName · $whenText"
        else ->
            "Found on your phone · $whenText"
    }
}
