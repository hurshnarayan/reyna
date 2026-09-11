package app.reyna.attribution

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationReaderTest {

    @Test
    fun `detects document attachments with emoji`() {
        val (name, hasAtt) = NotificationReader.detectAttachment(
            "📄 Additional Mathematics-I Classes for III sem Lateral entry students.pdf (1 page)"
        )
        assertTrue(hasAtt)
        assertEquals("Additional Mathematics-I Classes for III sem Lateral entry students.pdf", name)

        val (name2, hasAtt2) = NotificationReader.detectAttachment("📄 sample.pdf (1 page)")
        assertTrue(hasAtt2)
        assertEquals("sample.pdf", name2)
    }

    @Test
    fun `detects photo and media attachments`() {
        val (name1, hasAtt1) = NotificationReader.detectAttachment("📷 Photo")
        assertTrue(hasAtt1)
        assertEquals("", name1)

        val (name2, hasAtt2) = NotificationReader.detectAttachment("📷 4 photos")
        assertTrue(hasAtt2)
        assertEquals("", name2)

        val (name3, hasAtt3) = NotificationReader.detectAttachment("Chalega?")
        assertFalse(hasAtt3)
        assertEquals("", name3)
    }

    @Test
    fun `detects whatsapp system and summary notifications`() {
        assertTrue(NotificationReader.isSummary("WhatsApp", "52 messages from 50 chats"))
        assertTrue(NotificationReader.isSummary("WhatsApp", "Checking for new messages"))
        assertTrue(NotificationReader.isSummary("Harsh", "Ongoing voice call"))
        assertTrue(NotificationReader.isSummary("Yash", "Incoming voice call"))
        assertTrue(NotificationReader.isSummary("Yash", "Calling…"))
        assertTrue(NotificationReader.isSummary("Yash", "Ringing…"))

        assertFalse(NotificationReader.isSummary("Yash Bmsit", "📄 sample.pdf (1 page)"))
        assertFalse(NotificationReader.isSummary("BMSIT CIVIL", "📄 Dress Code.pdf (1 page)"))
        assertFalse(NotificationReader.isSummary("Tripuredh", "📷 Photo"))
    }

    @Test
    fun `cleans group chat name by removing message counter`() {
        assertEquals("BMSIT CIVIL 2025-2029", NotificationReader.cleanChatName("BMSIT CIVIL 2025-2029 (5 messages)"))
        assertEquals("BMSIT CIVIL 2025-2029", NotificationReader.cleanChatName("BMSIT CIVIL 2025-2029 (1 message)"))
        assertEquals("Direct Chat", NotificationReader.cleanChatName("Direct Chat"))
    }

    @Test
    fun `splits group sender correctly`() {
        val (sender1, msg1) = NotificationReader.splitGroupBody("~ Arun: 📄 Dress Code-Circular (1).pdf (1 page)")
        assertEquals("~ Arun", sender1)
        assertEquals("📄 Dress Code-Circular (1).pdf (1 page)", msg1)

        val (sender2, msg2) = NotificationReader.splitGroupBody("📷 Photo")
        assertEquals("", sender2)
        assertEquals("📷 Photo", msg2)
    }
}
