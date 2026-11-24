package app.reyna.attribution

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.GregorianCalendar
import java.util.TimeZone

/**
 * Ported from the Go suite in `internal/whatsapp/export/parse_test.go`. The two
 * parsers must agree, so the fixtures are deliberately identical — if a case is
 * added on one side, add it on the other.
 */
class ExportParserTest {

    private val utc: TimeZone = TimeZone.getTimeZone("UTC")
    private fun opts() = ExportParser.Options(defaultDayFirst = true, timeZone = utc)

    private fun at(y: Int, mo: Int, d: Int, h: Int, mi: Int, s: Int): Long =
        GregorianCalendar(utc).apply { clear(); set(y, mo - 1, d, h, mi, s) }.timeInMillis

    private fun one(line: String): ExportParser.Message {
        val res = ExportParser.parse(line, opts())
        assertEquals("expected exactly one message from: $line", 1, res.messages.size)
        return res.messages[0]
    }

    /**
     * Timestamp shapes real exports produce. These vary by platform, OS
     * version, locale and phone; a pattern that handles only the developer's
     * own device silently drops every message from everyone else's.
     */
    @Test
    fun `parses every header format`() {
        data class Case(val name: String, val line: String, val at: Long, val sender: String, val text: String)

        val cases = listOf(
            Case("iOS bracketed with seconds", "[12/08/2026, 21:14:03] Rahul Verma: hello",
                at(2026, 8, 12, 21, 14, 3), "Rahul Verma", "hello"),
            Case("Android dash separator", "12/08/2026, 21:14 - Rahul Verma: hello",
                at(2026, 8, 12, 21, 14, 0), "Rahul Verma", "hello"),
            Case("two-digit year", "[12/08/26, 21:14:03] Rahul: hello",
                at(2026, 8, 12, 21, 14, 3), "Rahul", "hello"),
            Case("12-hour PM", "13/08/2026, 9:14 PM - Rahul: hello",
                at(2026, 8, 13, 21, 14, 0), "Rahul", "hello"),
            Case("12-hour AM midnight", "13/08/2026, 12:05 AM - Rahul: hello",
                at(2026, 8, 13, 0, 5, 0), "Rahul", "hello"),
            // iOS 17+ emits U+202F before AM/PM. A pattern using \s fails on
            // every message from a recent iPhone.
            Case("narrow no-break space before PM", "[13/08/2026, 9:14:03 PM] Rahul: hello",
                at(2026, 8, 13, 21, 14, 3), "Rahul", "hello"),
            // iOS prefixes lines with a left-to-right mark, which breaks any
            // anchored pattern.
            Case("leading LTR mark", "‎[13/08/2026, 21:14:03] Rahul: hello",
                at(2026, 8, 13, 21, 14, 3), "Rahul", "hello"),
            Case("dot separators", "13.08.2026, 21:14 - Rahul: hello",
                at(2026, 8, 13, 21, 14, 0), "Rahul", "hello"),
            Case("sender name containing a colon", "[13/08/2026, 21:14:03] Dr: Mehta: hello there",
                at(2026, 8, 13, 21, 14, 3), "Dr", "Mehta: hello there"),
            // Splitting on the last colon would make the sender "check this out"
            // and lose the message.
            Case("body containing a colon", "[13/08/2026, 21:14:03] Rahul: check this out: https://x.com",
                at(2026, 8, 13, 21, 14, 3), "Rahul", "check this out: https://x.com"),
        )

        for (c in cases) {
            val m = one(c.line)
            assertEquals("${c.name}: time", c.at, m.postedAtMillis)
            assertEquals("${c.name}: sender", c.sender, m.sender)
            assertEquals("${c.name}: text", c.text, m.text)
        }
    }

    /**
     * Both platforms' attachment notation, including non-English exports.
     * Matching the literal English "(file attached)" would drop every
     * attachment from a Spanish or Hindi phone, which is exactly the data this
     * exists to recover.
     */
    @Test
    fun `parses attachments across locales`() {
        assertEquals("DOC-20260818-WA0001.pdf",
            one("[18/08/2026, 21:14:03] Mohit: <attached: DOC-20260818-WA0001.pdf>").attachmentName)
        assertEquals("DOC-20260818-WA0001.pdf",
            one("18/08/2026, 21:14 - Mohit: DOC-20260818-WA0001.pdf (file attached)").attachmentName)
        assertEquals("DOC-20260818-WA0001.pdf",
            one("18/08/2026, 21:14 - Mohit: DOC-20260818-WA0001.pdf (archivo adjunto)").attachmentName)
        assertEquals("DOC-20260818-WA0001.pdf",
            one("18/08/2026, 21:14 - Mohit: DOC-20260818-WA0001.pdf (फ़ाइल संलग्न)").attachmentName)
        assertEquals("Compiler Design Notes.pdf",
            one("[18/08/2026, 21:14:03] Mohit: <attached: Compiler Design Notes.pdf>").attachmentName)

        assertTrue(one("18/08/2026, 21:14 - Mohit: <Media omitted>").mediaOmitted)
        assertTrue(one("18/08/2026, 21:14 - Mohit: <Multimedia omitido>").mediaOmitted)

        // A sentence that happens to end in a parenthetical is not an attachment.
        val plain = one("18/08/2026, 21:14 - Mohit: see you at 8 (bring notes.pdf)")
        assertEquals("", plain.attachmentName)
        assertFalse(plain.mediaOmitted)
    }

    /**
     * A single line cannot settle day-vs-month, but one date above 12 anywhere
     * in the file rules that position out as a month. Getting this wrong shifts
     * every date by up to eleven months while looking entirely plausible.
     */
    @Test
    fun `resolves date order from the whole file`() {
        val dayFirst = ExportParser.parse(
            "[13/08/2026, 10:00:00] A: x\n[05/08/2026, 10:00:00] A: y\n",
            ExportParser.Options(defaultDayFirst = false, timeZone = utc),
        )
        assertTrue("the file proves day-first, overriding the default", dayFirst.dateOrder.dayFirst)
        assertTrue(dayFirst.dateOrder.proven)

        val monthFirst = ExportParser.parse(
            "[08/13/2026, 10:00:00] A: x\n[08/05/2026, 10:00:00] A: y\n",
            ExportParser.Options(defaultDayFirst = true, timeZone = utc),
        )
        assertFalse("the file proves month-first", monthFirst.dateOrder.dayFirst)
        assertTrue(monthFirst.dateOrder.proven)

        val ambiguous = ExportParser.parse(
            "[05/08/2026, 10:00:00] A: x\n[06/09/2026, 10:00:00] A: y\n",
            ExportParser.Options(defaultDayFirst = true, timeZone = utc),
        )
        assertFalse("nothing in this file settles it", ambiguous.dateOrder.proven)
        assertTrue(ambiguous.dateOrder.dayFirst)
        assertTrue("must tell the caller it is an assumption",
            ambiguous.dateOrder.toString().contains("assumed"))
    }

    /** Continuation lines belong to their message, and are not failures. */
    @Test
    fun `joins multi-line messages`() {
        val res = ExportParser.parse(
            "[18/08/2026, 21:14:03] Mohit: first line\nsecond line\nthird line\n" +
                "[18/08/2026, 21:15:00] Priya: next message\n",
            opts(),
        )
        assertEquals(2, res.messages.size)
        assertEquals("first line\nsecond line\nthird line", res.messages[0].text)
        assertEquals("continuations are not parse failures", 0, res.unparsedLines)
    }

    /**
     * WhatsApp's own notices must not be attributed to a person. Reading the
     * encryption banner as a sender would invent a contact that could then be
     * cited as one.
     */
    @Test
    fun `does not invent senders for system messages`() {
        val res = ExportParser.parse(
            "[18/08/2026, 21:14:03] Messages and calls are end-to-end encrypted. No one outside of this chat can read them.\n" +
                "[18/08/2026, 21:15:00] Mohit: real message\n" +
                "[18/08/2026, 21:16:00] Rahul Verma added Priya\n",
            opts(),
        )
        assertEquals(3, res.messages.size)
        assertEquals("", res.messages[0].sender)
        assertTrue(res.messages[0].system)
        assertEquals("Mohit", res.messages[1].sender)
        assertEquals("group-add notice has no sender", "", res.messages[2].sender)
    }

    /**
     * One unrecognised line must cost one message, never the file. Someone
     * importing three years of history cannot lose all of it to a single odd
     * line.
     */
    @Test
    fun `survives garbage`() {
        val res = ExportParser.parse(
            "total nonsense with no timestamp at the very top\n" +
                "[18/08/2026, 21:14:03] Mohit: good message\n" +
                "[99/99/9999, 99:99:99] Nobody: impossible date\n" +
                "[18/08/2026, 21:15:00] Priya: another good message\n",
            opts(),
        )
        assertEquals("the two good messages survive", 2, res.messages.size)
        assertNotEquals("bad lines are reported, not silently dropped", 0, res.unparsedLines)
    }

    /** End to end on something shaped like a real study-group export. */
    @Test
    fun `parses a realistic export`() {
        val res = ExportParser.parse(
            """
            [15/08/2026, 09:12:44] Messages and calls are end-to-end encrypted.
            [15/08/2026, 09:13:01] Mohit Sharma: guys anyone has the compiler notes
            [15/08/2026, 09:15:22] Priya R: <attached: Compiler_Design_Module3.pdf>
            [15/08/2026, 09:15:23] Priya R: here
            [16/08/2026, 23:47:10] Rakesh: DOC-20260816-WA0007.pdf (file attached)
            [16/08/2026, 23:47:55] Mohit Sharma: thanks
            this is a second line of the same message
            [17/08/2026, 08:02:00] Priya R: <Media omitted>
            """.trimIndent(),
            opts(),
        )
        assertEquals(0, res.unparsedLines)
        assertTrue("the 15th, 16th and 17th settle the order", res.dateOrder.proven)
        assertTrue(res.dateOrder.dayFirst)

        val att = res.attachments()
        assertEquals(3, att.size)
        assertEquals("Compiler_Design_Module3.pdf", att[0].attachmentName)
        assertEquals("Priya R", att[0].sender)
        assertEquals(at(2026, 8, 15, 9, 15, 22), att[0].postedAtMillis)
        assertEquals("DOC-20260816-WA0007.pdf", att[1].attachmentName)
        assertEquals("Rakesh", att[1].sender)
        assertTrue(att[2].mediaOmitted)
        assertEquals("Priya R", att[2].sender)
    }
}
