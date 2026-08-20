package app.reyna.search

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Search is the product surface: a file Reyna holds but cannot surface is, to
 * the user, a file it lost. These tests pin the behaviour that makes half-
 * remembered queries work, and the ranking that decides which of many matches
 * is worth showing first.
 */
class FuzzySearchTest {

    @Test
    fun `matches a subsequence, not just a substring`() {
        // How people actually search: fragments of a name, in order, with gaps.
        assertNotNull(FuzzySearch.match("cdl", "Compiler_Design_Lab.pdf"))
        assertNotNull(FuzzySearch.match("complab", "Compiler_Design_Lab.pdf"))
        assertNotNull(FuzzySearch.match("oskernel", "OS_Kernel_Notes.pdf"))
    }

    @Test
    fun `rejects characters out of order`() {
        // Fuzzy is not "contains these letters somewhere". Order still matters,
        // or every query would match every file.
        assertNull(FuzzySearch.match("bal", "Compiler_Design_Lab.pdf"))
        assertNull(FuzzySearch.match("zzz", "Compiler_Design_Lab.pdf"))
    }

    @Test
    fun `an exact substring outranks a scattered match`() {
        val exact = FuzzySearch.match("compiler", "Compiler_Lab.pdf")!!
        val scattered = FuzzySearch.match("compiler", "Computer_Physics_Lab_Reference.pdf")
        if (scattered != null) {
            assertTrue(
                "an exact hit must beat a scatter, or search feels random",
                exact.score > scattered.score,
            )
        }
    }

    @Test
    fun `word boundaries beat mid-word matches`() {
        // "lab" starting a word is a better hit than "lab" buried inside one.
        val boundary = FuzzySearch.match("lab", "Compiler_Lab_Manual.pdf")!!
        val midWord = FuzzySearch.match("lab", "Syllabus_2026.pdf")!!
        assertTrue(
            "boundary=${boundary.score} midWord=${midWord.score}",
            boundary.score > midWord.score,
        )
    }

    @Test
    fun `reports the positions that matched`() {
        val m = FuzzySearch.match("comp", "Compiler_Lab.pdf")!!
        assertEquals(listOf(0, 1, 2, 3), m.positions)
        // Highlighting is what makes a fuzzy hit legible rather than mysterious,
        // so the spans have to be exact.
        val scattered = FuzzySearch.match("cdl", "Compiler_Design_Lab.pdf")!!
        assertEquals(3, scattered.positions.size)
        assertTrue("positions must ascend", scattered.positions.zipWithNext().all { it.first < it.second })
    }

    @Test
    fun `an empty query matches everything`() {
        // A blank search box shows the unfiltered list, never an empty screen.
        val m = FuzzySearch.match("", "anything.pdf")
        assertNotNull(m)
        assertEquals(0, m!!.score)
    }

    @Test
    fun `matching is case insensitive`() {
        assertNotNull(FuzzySearch.match("COMPILER", "compiler_lab.pdf"))
        assertNotNull(FuzzySearch.match("compiler", "COMPILER_LAB.PDF"))
    }

    @Test
    fun `suggests corrections only for words the user actually has`() {
        val vocab = FileSearch.vocabulary(sampleFiles())
        val suggestions = FuzzySearch.suggestions("compilr", vocab)
        assertTrue("expected 'compiler' in $suggestions", suggestions.contains("compiler"))

        // A suggestion Reyna cannot deliver on is worse than none: every one
        // must return results when tapped.
        assertTrue(
            "suggested a word not in the library: $suggestions",
            suggestions.all { it in vocab },
        )
    }

    @Test
    fun `offers nothing for a query nothing resembles`() {
        val vocab = FileSearch.vocabulary(sampleFiles())
        assertTrue(FuzzySearch.suggestions("qqqqqqzz", vocab).isEmpty())
    }

    @Test
    fun `a transposition costs one edit, not two`() {
        // Swapping two letters is the most common typo there is. Plain
        // Levenshtein charges 2 for it, which puts the one suggestion the user
        // obviously wanted outside a one-edit tolerance.
        assertEquals(1, FuzzySearch.editDistance("mohti", "mohit"))
        assertEquals(1, FuzzySearch.editDistance("teh", "the"))
        assertEquals(1, FuzzySearch.editDistance("compiler", "compilre"))

        val vocab = FileSearch.vocabulary(sampleFiles())
        assertTrue(
            "a transposed name must still be suggestible",
            FuzzySearch.suggestions("mohti", vocab).contains("mohit"),
        )
    }

    @Test
    fun `edit distance is bounded`() {
        assertEquals(0, FuzzySearch.editDistance("torrent", "torrent"))
        assertEquals(1, FuzzySearch.editDistance("torrend", "torrent"))
        assertTrue(FuzzySearch.editDistance("aaaaaaaa", "bbbbbbbb", max = 3) > 3)
    }

    @Test
    fun `tokenizes filenames on every separator they actually use`() {
        val tokens = FuzzySearch.tokenize("Compiler_Design-Lab.Manual v2.pdf")
        assertTrue(tokens.containsAll(listOf("compiler", "design", "lab", "manual", "pdf")))
    }

    // ── FileSearch ──

    @Test
    fun `searches name, sender and chat from one box`() {
        val files = sampleFiles()
        assertTrue(FileSearch.search("compiler", files).isNotEmpty())
        assertTrue("should find files by who shared them", FileSearch.search("mohit", files).isNotEmpty())
        assertTrue("should find files by which chat", FileSearch.search("sem 5", files).isNotEmpty())
    }

    @Test
    fun `will not match a sender it would refuse to name`() {
        // A file attributed too weakly to display a sender must not be findable
        // by that sender either, or the result shows no name and reads as a bug.
        val files = listOf(
            SearchableFile(1, "DOC-20260818-WA0041.pdf", "Rakesh", "Sem 5 CS", "18 Aug", 0.45),
        )
        assertTrue(
            "a 0.45 guess is not strong enough to be searchable by name",
            FileSearch.search("rakesh", files).isEmpty(),
        )
        // The same file is still reachable by chat and by filename.
        assertTrue(FileSearch.search("sem 5", files).isNotEmpty())
        assertTrue(FileSearch.search("wa0041", files).isNotEmpty())
    }

    @Test
    fun `strict mode only matches substrings`() {
        val files = sampleFiles()
        // The scattered query works fuzzily and not strictly, which is the
        // whole reason the toggle exists.
        assertTrue(FileSearch.search("cdl", files, fuzzy = true).isNotEmpty())
        assertTrue(FileSearch.search("cdl", files, fuzzy = false).isEmpty())
        assertTrue(FileSearch.search("compiler", files, fuzzy = false).isNotEmpty())
    }

    @Test
    fun `ranks the better filename match first`() {
        val files = listOf(
            SearchableFile(1, "Computer_Networks_Reference.pdf", "Priya", "Sem 5 CS", "1 Aug", 1.0),
            SearchableFile(2, "Compiler_Lab_Manual.pdf", "Mohit", "Sem 5 CS", "18 Aug", 1.0),
        )
        val hits = FileSearch.search("compiler", files)
        assertEquals("Compiler_Lab_Manual.pdf", hits.first().file.fileName)
    }

    @Test
    fun `an empty query returns the whole library`() {
        val files = sampleFiles()
        assertEquals(files.size, FileSearch.search("", files).size)
    }

    @Test
    fun `chat facets count the results, not the library`() {
        val files = sampleFiles()
        val hits = FileSearch.search("compiler", files)
        val facets = FileSearch.chatFacets(hits)
        // A chip promising twelve that then shows three is worse than no chip.
        assertEquals(hits.size, facets.sumOf { it.second })
    }

    private fun sampleFiles() = listOf(
        SearchableFile(1, "Compiler_Design_Lab.pdf", "Mohit", "Sem 5 CS", "18 Aug", 0.95),
        SearchableFile(2, "OS_Kernel_Notes.pdf", "Priya", "Sem 5 CS", "17 Aug", 0.85),
        SearchableFile(3, "DOC-20260818-WA0041.pdf", null, "Sem 5 CS", "18 Aug", 0.45),
        SearchableFile(4, "DBMS_PYQ_2025.pdf", "Rakesh", "Hostel Block C", "9 Aug", 0.95),
    )
}
