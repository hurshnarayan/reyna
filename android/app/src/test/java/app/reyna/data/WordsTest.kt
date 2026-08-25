package app.reyna.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The phone uses this to decide which unsent files to push ahead of a
 * question. Matching the wrong ones means the server searches without the
 * document the question was about.
 */
class WordsTest {

    @Test
    fun splitsLetterDigitBoundary() {
        assertEquals(
            listOf("module", "4", "part", "1", "arrays", "pptx"),
            Words.of("Module4_part1_Arrays.pptx"),
        )
    }

    @Test
    fun odeDoesNotMatchDiodes() {
        assertFalse(Words.matchText("MODULE 1-SEMICONDUCTOR DIODES.pptx", "ode"))
        assertTrue(Words.matchText("2CSE Module 1 ODE of first order.pdf", "ode"))
    }

    @Test
    fun adjacencySeparatesModule1FromPart1() {
        val tokens = listOf("module", "1")
        assertTrue(
            Words.score("Module 1-PPT-1.pptx", tokens) >
                Words.score("Module4_part1_Arrays.pptx", tokens)
        )
    }

    @Test
    fun prefixReachesPluralOnly() {
        assertTrue(Words.matchText("first order differentials.pdf", "differential"))
        assertFalse(Words.matchText("diodes.pdf", "die"))
    }

    @Test
    fun scoreIsZeroWhenNothingMatches() {
        assertEquals(0, Words.score("Calendar of Events.pdf", listOf("module", "ode")))
    }
}
