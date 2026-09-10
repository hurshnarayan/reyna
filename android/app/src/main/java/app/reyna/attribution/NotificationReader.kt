package app.reyna.attribution

import android.app.Notification
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import android.util.Log
import androidx.core.app.NotificationCompat
import app.reyna.data.Repo
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * Reads WhatsApp notifications to learn who shared a file and when.
 *
 * A file on disk carries no sender. The notification does. This is the only
 * mechanism Android offers for attributing a file to the message that
 * delivered it, and it is why the app requests notification access at all.
 *
 * Scope is deliberately narrow, and the app must be able to say so honestly to
 * users and to Play review: only WhatsApp notifications are read, only the
 * sender name, conversation name and timestamp are extracted, and none of it
 * leaves the device.
 *
 * Two properties shape everything downstream. Notification history **cannot be
 * backfilled** — a listener only sees what is posted after it is enabled, so
 * every hour without permission is attribution lost permanently, which is why
 * onboarding asks for this first. And the listener is the least reliable of
 * Reyna's attribution sources: muted chats, Do Not Disturb, an open chat
 * window, and aggressive OEM battery managers can all suppress it. That is why
 * it is one source of four rather than the design.
 */
class NotificationReader : NotificationListenerService() {

    /** What one WhatsApp notification told us. */
    data class Observed(
        val chatName: String,
        val isGroup: Boolean,
        val senderName: String,
        /** Stable across contact renames, unlike the display name. */
        val senderKey: String,
        /** Exact post time from the system, not our own clock. */
        val postedAtMillis: Long,
        val text: String,
        /** Sometimes carries the chat identifier; far more stable than a name. */
        val shortcutId: String?,
        /**
         * How the fields were obtained. A MessagingStyle read is structured and
         * trustworthy; the fallback is a string split and is treated as such.
         */
        val source: Source,
    ) {
        enum class Source { MESSAGING_STYLE, EXTRAS_FALLBACK }

        /**
         * Ceiling on any attribution built from this observation.
         *
         * The fallback path parses "Sender: message" out of a title and body,
         * which is guesswork: a display name containing a colon, or a summary
         * notification covering several chats, both produce a plausible but
         * wrong sender. Capping below [Attribution.MIN_NAMED] means such a
         * guess can inform ranking but can never put a name on screen.
         */
        val confidenceCeiling: Double
            get() = when (source) {
                Source.MESSAGING_STYLE -> 0.85
                Source.EXTRAS_FALLBACK -> 0.60
            }
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        if (sbn.packageName !in WATCHED_PACKAGES) return
        val observed = extract(sbn) ?: return
        // Stored unconditionally, whether or not a file ever turns up for it.
        // A file downloaded hours later still needs this event to exist, and
        // the join runs in both directions for exactly that reason.
        scope.launch {
            runCatching { Repo.get(applicationContext).onNotification(observed) }
                .onFailure { Log.w(TAG, "could not record notification: ${it.message}") }
        }
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    companion object {
        private const val TAG = "ReynaNotif"
        private val WATCHED_PACKAGES = setOf("com.whatsapp", "com.whatsapp.w4b")

        /**
         * Pulls what we need out of a notification, preferring the structured
         * MessagingStyle payload and falling back to the raw extras.
         */
        fun extract(sbn: StatusBarNotification): Observed? {
            val n = sbn.notification ?: return null
            if ((n.flags and Notification.FLAG_GROUP_SUMMARY) != 0) return null

            val extras = n.extras ?: return null

            val style = try {
                NotificationCompat.MessagingStyle
                    .extractMessagingStyleFromNotification(n)
            } catch (e: Throwable) {
                Log.w(TAG, "MessagingStyle extraction failed: ${e.message}")
                null
            }

            val shortcut = try { n.shortcutId } catch (_: Throwable) { null }

            if (style != null) {
                val last = style.messages.lastOrNull()
                val person = last?.person
                val rawChat = style.conversationTitle?.toString()
                    ?: extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
                val text = last?.text?.toString().orEmpty()
                val chatName = cleanChatName(rawChat)

                if (chatName.isBlank() || chatName.equals("WhatsApp", ignoreCase = true)) return null
                if (isSummary(chatName, text)) return null

                val isGroup = style.isGroupConversation
                val rawSender = person?.name?.toString().orEmpty()
                val effectiveSender = when {
                    rawSender.isNotBlank() && !rawSender.equals("WhatsApp", ignoreCase = true) -> rawSender
                    !isGroup && chatName.isNotBlank() && !chatName.equals("WhatsApp", ignoreCase = true) -> chatName
                    else -> ""
                }

                if (effectiveSender.isBlank() && chatName.isBlank()) return null

                return Observed(
                    chatName = chatName,
                    isGroup = isGroup,
                    senderName = effectiveSender,
                    senderKey = person?.key.orEmpty(),
                    // The message's own timestamp beats the notification's when
                    // present: an update to an existing notification re-posts
                    // with a new postTime but the message is unchanged.
                    postedAtMillis = last?.timestamp ?: sbn.postTime,
                    text = text,
                    shortcutId = shortcut,
                    source = Observed.Source.MESSAGING_STYLE,
                )
            }

            // Fallback: older WhatsApp builds, and summary notifications that
            // carry no MessagingStyle at all.
            val rawTitle = extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
            val body = extras.getCharSequence(Notification.EXTRA_TEXT)?.toString().orEmpty()
            if (rawTitle.isEmpty() && body.isEmpty()) return null

            val title = cleanChatName(rawTitle)
            if (title.isBlank() || title.equals("WhatsApp", ignoreCase = true)) return null
            if (isSummary(title, body)) return null

            val (sender, message) = splitGroupBody(body)
            val isGroup = sender.isNotEmpty()
            val effectiveSender = when {
                sender.isNotBlank() && !sender.equals("WhatsApp", ignoreCase = true) -> sender
                !isGroup && title.isNotBlank() && !title.equals("WhatsApp", ignoreCase = true) -> title
                else -> ""
            }

            if (effectiveSender.isBlank() && title.isBlank()) return null

            return Observed(
                chatName = title,
                isGroup = isGroup,
                senderName = effectiveSender,
                senderKey = "",
                postedAtMillis = sbn.postTime,
                text = message,
                shortcutId = shortcut,
                source = Observed.Source.EXTRAS_FALLBACK,
            )
        }

        /** Removes trailing message counter like " (5 messages)" from group titles. */
        fun cleanChatName(raw: String): String =
            raw.replace(Regex("""\s*\(\d+\s+messages?\)$""", RegexOption.IGNORE_CASE), "").trim()

        /**
         * Detects WhatsApp system notifications, call status, and multi-chat summaries
         * that do not represent individual messages.
         */
        fun isSummary(title: String, text: String): Boolean {
            val t = title.trim()
            val b = text.trim()
            if (t.equals("WhatsApp", ignoreCase = true)) return true
            if (b.matches(Regex("""^\d+\s+messages?\s+from\s+\d+\s+chats?.*""", RegexOption.IGNORE_CASE))) return true
            if (b.matches(Regex("""^\d+\s+new\s+messages?.*""", RegexOption.IGNORE_CASE))) return true
            if (b.equals("Checking for new messages", ignoreCase = true)) return true
            if (b.startsWith("Ongoing voice call", ignoreCase = true) ||
                b.startsWith("Incoming voice call", ignoreCase = true) ||
                b.startsWith("Calling…", ignoreCase = true) ||
                b.startsWith("Ringing…", ignoreCase = true) ||
                b.startsWith("Missed voice call", ignoreCase = true) ||
                b.startsWith("Missed video call", ignoreCase = true) ||
                b.startsWith("Deleting messages…", ignoreCase = true) ||
                t.startsWith("Deleting messages…", ignoreCase = true)
            ) return true
            return false
        }

        /**
         * Parses notification text for attachments (documents or media).
         * Returns (attachmentName, hasAttachment).
         */
        fun detectAttachment(text: String): Pair<String, Boolean> {
            val trimmed = text.trim()
            // Document with emoji: e.g. "📄 Additional Mathematics.pdf (1 page)"
            val docMatch = Regex("""^📄\s*(.+?)(?:\s*\(\d+\s+pages?\))?$""").find(trimmed)
            if (docMatch != null) {
                val fileName = docMatch.groupValues[1].trim()
                return fileName to true
            }
            // Document without emoji but ending with doc extension
            val extMatch = Regex("""^(.+?\.(?:pdf|docx?|pptx?|xlsx?|txt|csv|zip|epub|rtf))(?:\s*\(.*\))?$""", RegexOption.IGNORE_CASE).find(trimmed)
            if (extMatch != null) {
                val fileName = extMatch.groupValues[1].trim()
                return fileName to true
            }
            // Photos/Images
            if (trimmed.contains("📷") ||
                trimmed.matches(Regex(""".*\b(?:\d+\s+)?(?:photos?|images?)\b.*""", RegexOption.IGNORE_CASE))
            ) {
                return "" to true
            }
            // Other media indicators
            if (trimmed.contains("🎥") || trimmed.contains("🎤") ||
                trimmed.contains("Audio") || trimmed.contains("Video")
            ) {
                return "" to true
            }
            return "" to false
        }

        /**
         * Splits "Sender: message" from a group notification body.
         */
        internal fun splitGroupBody(body: String): Pair<String, String> {
            val idx = body.indexOf(": ")
            if (idx <= 0) return "" to body
            val candidate = body.substring(0, idx).trim()
            if (candidate.length > 60 || candidate.split(" ").size > 6) return "" to body
            return candidate to body.substring(idx + 2)
        }
    }
}
