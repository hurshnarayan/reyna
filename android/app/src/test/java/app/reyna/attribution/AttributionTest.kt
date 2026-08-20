package app.reyna.attribution

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.GregorianCalendar
import java.util.TimeZone
import java.util.concurrent.TimeUnit

/**
 * The join is the product. These tests pin what Reyna is allowed to claim, not
 * just what it computes — the failure mode that matters is a confident wrong
 * sender, which no user can detect.
 */
class AttributionTest {

    private val utc: TimeZone = TimeZone.getTimeZone("UTC")

    private fun at(y: Int, mo: Int, d: Int, h: Int, mi: Int): Long =
        GregorianCalendar(utc).apply { clear(); set(y, mo - 1, d, h, mi, 0) }.timeInMillis

    private fun event(
        id: Long,
        postedAt: Long,
        sender: String = "Mohit",
        attachment: String = "",
        text: String = "",
    ) = Attribution.Event(
        id = id, chatKey = "sem5", chatName = "Sem 5 CS",
        senderKey = "k$id", senderDisplay = sender,
        postedAtMillis = postedAt, text = text,
        attachmentName = attachment,
        hasAttachment = attachment.isNotEmpty() || text.contains("attach"),
    )

    private fun file(name: String, mtime: Long, isSent: Boolean = false) =
        Attribution.CapturedFile(id = 1, diskName = name, mtimeMillis = mtime, isSent = isSent)

    /** A file the user shared themselves is certain, and costs nothing to know. */
    @Test
    fun `sent folder is certain`() {
        val r = Attribution.attribute(file("notes.pdf", at(2026, 8, 18, 21, 0), isSent = true), emptyList())
        assertEquals(1.0, r.confidence, 0.001)
        assertEquals(Attribution.Method.SELF_SENT, r.method)
        assertTrue(r.canName)
    }

    /** An event naming the exact file beats everything else. */
    @Test
    fun `exact filename match is strongest`() {
        val posted = at(2026, 8, 18, 21, 14)
        val r = Attribution.attribute(
            file("Compiler_Notes.pdf", posted + TimeUnit.MINUTES.toMillis(2)),
            listOf(
                event(1, posted, sender = "Priya", attachment = "Compiler_Notes.pdf"),
                event(2, posted, sender = "Rakesh", attachment = "Something_Else.pdf"),
            ),
            zone = utc,
        )
        assertEquals(1L, r.best?.eventId)
        assertEquals(0.95, r.confidence, 0.001)
        assertTrue(r.canName)
    }

    /**
     * The same file downloaded days later is still that file. A user who taps
     * download on Friday for something posted Monday must not lose the sender.
     */
    @Test
    fun `exact match survives a long delay but is rated lower`() {
        val posted = at(2026, 8, 18, 21, 14)
        val r = Attribution.attribute(
            file("Compiler_Notes.pdf", posted + TimeUnit.DAYS.toMillis(3)),
            listOf(event(1, posted, attachment = "Compiler_Notes.pdf")),
            zone = utc,
        )
        assertEquals(0.85, r.confidence, 0.001)
        assertTrue("still strong enough to name", r.canName)
    }

    /**
     * When WhatsApp renames the file, the embedded date is all we have. One
     * candidate that day is good enough to name; several is a guess.
     */
    @Test
    fun `date-unique names, date-ambiguous does not`() {
        val posted = at(2026, 8, 18, 21, 14)
        val unique = Attribution.attribute(
            file("DOC-20260818-WA0007.pdf", posted + TimeUnit.MINUTES.toMillis(30)),
            listOf(event(1, posted, sender = "Priya", attachment = "x.pdf")),
            zone = utc,
        )
        assertEquals(Attribution.Method.DATE_UNIQUE, unique.method)
        assertTrue("a single candidate that day may be named", unique.canName)

        val ambiguous = Attribution.attribute(
            file("DOC-20260818-WA0007.pdf", posted + TimeUnit.MINUTES.toMillis(30)),
            listOf(
                event(1, posted, sender = "Priya", attachment = "x.pdf"),
                event(2, at(2026, 8, 18, 9, 0), sender = "Rakesh", attachment = "y.pdf"),
                event(3, at(2026, 8, 18, 14, 0), sender = "Mohit", attachment = "z.pdf"),
            ),
            zone = utc,
        )
        assertFalse(
            "three candidates on the same day is a guess and must not be named",
            ambiguous.canName,
        )
        assertEquals(Attribution.Method.TIME_WINDOW, ambiguous.method)
        assertTrue("still worth surfacing the chat", ambiguous.confidence >= 0.30)
    }

    /** Nothing matched is a legitimate answer, not an error to hide. */
    @Test
    fun `no candidates yields no attribution`() {
        val r = Attribution.attribute(
            file("DOC-20260818-WA0007.pdf", at(2026, 8, 18, 21, 14)),
            listOf(event(1, at(2026, 1, 1, 9, 0), attachment = "unrelated.pdf")),
            zone = utc,
        )
        assertNull(r.best)
        assertEquals(0.0, r.confidence, 0.001)
        assertFalse(r.canName)
    }

    /**
     * Every candidate is kept, not only the winner, so a chat export imported
     * later can promote a better match without the earlier reasoning being lost.
     */
    @Test
    fun `keeps losing candidates on record`() {
        val posted = at(2026, 8, 18, 21, 14)
        val r = Attribution.attribute(
            file("DOC-20260818-WA0007.pdf", posted),
            listOf(
                event(1, posted, sender = "Priya", attachment = "a.pdf"),
                event(2, posted + TimeUnit.MINUTES.toMillis(1), sender = "Rakesh", attachment = "b.pdf"),
            ),
            zone = utc,
        )
        assertTrue("alternatives must survive for later revision", r.candidates.size >= 2)
    }

    /** The WhatsApp name carries a date and an intra-day ordering. */
    @Test
    fun `reads whatsapp generated names`() {
        assertEquals(Triple(2026, 8, 18), Attribution.waNameDate("DOC-20260818-WA0007.pdf"))
        assertEquals(7, Attribution.waNameCounter("DOC-20260818-WA0007.pdf"))
        assertEquals(Triple(2026, 8, 18), Attribution.waNameDate("IMG-20260818-WA0012.jpg"))
        assertTrue(
            "ordering within a day works with no other source",
            Attribution.waNameCounter("DOC-20260818-WA0003.pdf")!! <
                Attribution.waNameCounter("DOC-20260818-WA0007.pdf")!!,
        )
        assertNull("a user-named file has no embedded date", Attribution.waNameDate("Compiler_Notes.pdf"))
    }

    /**
     * The rule the whole design exists to enforce: below the threshold, Reyna
     * says where and when, never who. A wrong name is worse than no name
     * because the user cannot tell it is wrong.
     */
    @Test
    fun `never names anyone below the threshold`() {
        assertEquals(
            "Mohit · 18 August",
            Attribution.describe(0.95, "Mohit", "Sem 5 CS", "18 August"),
        )
        assertEquals(
            "a weak guess must fall back to the chat",
            "Sem 5 CS · 18 August",
            Attribution.describe(0.45, "Mohit", "Sem 5 CS", "18 August"),
        )
        assertEquals(
            "nothing known at all",
            "Found on your phone · 18 August",
            Attribution.describe(0.0, null, null, "18 August"),
        )
        assertEquals(
            "a name we hold but cannot stand behind is still withheld",
            "Sem 5 CS · 18 August",
            Attribution.describe(0.69, "Mohit", "Sem 5 CS", "18 August"),
        )
    }

    /** The Kotlin and Go thresholds must not drift apart. */
    @Test
    fun `threshold matches the server`() {
        assertEquals(0.70, Attribution.MIN_NAMED, 0.0001)
    }
}
