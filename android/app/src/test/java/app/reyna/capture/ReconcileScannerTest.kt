package app.reyna.capture

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class ReconcileScannerTest {
    @get:Rule val temp = TemporaryFolder()

    @Test
    fun scanFindsSupportedFilesInNestedDirectories() {
        val root = temp.newFolder("WhatsApp Documents")
        val nested = root.resolve("Private/course").apply { mkdirs() }
        val topLevel = root.resolve("top.pdf").apply {
            writeText("top")
            setLastModified(1L)
        }
        val nestedFile = nested.resolve("notes.docx").apply {
            writeText("nested")
            setLastModified(1L)
        }
        nested.resolve("ignored.bin").writeText("ignored")

        val found = ReconcileScanner(isKnown = { _, _ -> false }, scanRoots = { listOf(root) }).scan()

        assertEquals(setOf(topLevel.canonicalPath, nestedFile.canonicalPath), found.map { it.file.canonicalPath }.toSet())
        assertTrue(found.all { it.mtimeMillis == 1L })
    }
}
