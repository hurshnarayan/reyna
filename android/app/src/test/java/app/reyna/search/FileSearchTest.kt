package app.reyna.search

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class FileSearchTest {

    private fun sampleFile(
        id: Long,
        name: String,
        mtime: Long = 1000L,
        postedAt: Long = 1000L,
        sizeBytes: Long = 1024L,
        isImage: Boolean = false,
    ) = SearchableFile(
        id = id,
        fileName = name,
        senderName = "Tester",
        chatName = "Group",
        whenText = "1 hour ago",
        confidence = 0.85,
        isImage = isImage,
        path = "/fake/$name",
        sizeBytes = sizeBytes,
        mtime = mtime,
        postedAt = postedAt,
    )

    @Test
    fun `sorts by captured latest by default`() {
        val f1 = sampleFile(1, "Old_Capture.pdf", mtime = 1000L)
        val f2 = sampleFile(2, "Middle_Capture.pdf", mtime = 2000L)
        val f3 = sampleFile(3, "Newest_Capture.pdf", mtime = 3000L)

        val hits = listOf(FileHit(f1, 0, emptyList()), FileHit(f2, 0, emptyList()), FileHit(f3, 0, emptyList()))

        val sortedDesc = FileSearch.sortFiles(hits, SortMode.LATEST, ascending = false)
        assertEquals(listOf(3L, 2L, 1L), sortedDesc.map { it.file.id })

        val sortedAsc = FileSearch.sortFiles(hits, SortMode.LATEST, ascending = true)
        assertEquals(listOf(1L, 2L, 3L), sortedAsc.map { it.file.id })
    }

    @Test
    fun `sorts by date`() {
        val f1 = sampleFile(1, "A.pdf", postedAt = 5000L)
        val f2 = sampleFile(2, "B.pdf", postedAt = 10000L)
        val f3 = sampleFile(3, "C.pdf", postedAt = 1000L)

        val hits = listOf(FileHit(f1, 0, emptyList()), FileHit(f2, 0, emptyList()), FileHit(f3, 0, emptyList()))

        val sorted = FileSearch.sortFiles(hits, SortMode.DATE, ascending = false)
        assertEquals(listOf(2L, 1L, 3L), sorted.map { it.file.id })
    }

    @Test
    fun `sorts alphabetically by name`() {
        val f1 = sampleFile(1, "Zebra.pdf")
        val f2 = sampleFile(2, "apple.pdf")
        val f3 = sampleFile(3, "Banana.pdf")

        val hits = listOf(FileHit(f1, 0, emptyList()), FileHit(f2, 0, emptyList()), FileHit(f3, 0, emptyList()))

        val sortedAsc = FileSearch.sortFiles(hits, SortMode.NAME, ascending = true)
        assertEquals(listOf(2L, 3L, 1L), sortedAsc.map { it.file.id })

        val sortedDesc = FileSearch.sortFiles(hits, SortMode.NAME, ascending = false)
        assertEquals(listOf(1L, 3L, 2L), sortedDesc.map { it.file.id })
    }

    @Test
    fun `sorts by size`() {
        val f1 = sampleFile(1, "Small.pdf", sizeBytes = 100L)
        val f2 = sampleFile(2, "Huge.pdf", sizeBytes = 50_000_000L)
        val f3 = sampleFile(3, "Medium.pdf", sizeBytes = 5_000L)

        val hits = listOf(FileHit(f1, 0, emptyList()), FileHit(f2, 0, emptyList()), FileHit(f3, 0, emptyList()))

        val sortedDesc = FileSearch.sortFiles(hits, SortMode.SIZE, ascending = false)
        assertEquals(listOf(2L, 3L, 1L), sortedDesc.map { it.file.id })
    }

    @Test
    fun `sorts by kind`() {
        val fDoc = sampleFile(1, "document.docx")
        val fImg = sampleFile(2, "photo.jpg", isImage = true)
        val fPdf = sampleFile(3, "syllabus.pdf")
        val fZip = sampleFile(4, "archive.zip")

        val hits = listOf(FileHit(fDoc, 0, emptyList()), FileHit(fImg, 0, emptyList()), FileHit(fPdf, 0, emptyList()), FileHit(fZip, 0, emptyList()))

        val sorted = FileSearch.sortFiles(hits, SortMode.KIND, ascending = true)
        // Images (0), PDFs (1), Documents (2), ..., Archives (5)
        assertEquals(FileKind.IMAGE, sorted[0].file.kind)
        assertEquals(FileKind.PDF, sorted[1].file.kind)
        assertEquals(FileKind.DOCUMENT, sorted[2].file.kind)
        assertEquals(FileKind.ARCHIVE, sorted[3].file.kind)
    }

    @Test
    fun `formats file size correctly`() {
        assertEquals("500 B", sampleFile(1, "a.pdf", sizeBytes = 500L).formattedSize)
        assertEquals("20 KB", sampleFile(1, "a.pdf", sizeBytes = 20 * 1024L).formattedSize)
        assertTrue(sampleFile(1, "a.pdf", sizeBytes = (2.5 * 1024 * 1024).toLong()).formattedSize.startsWith("2.5 MB"))
    }
}
