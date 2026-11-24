package app.reyna.data

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SmallTalkTest {

    @Test
    fun greetingsAreSmallTalk() {
        for (s in listOf("hi", "Hello", "hey there", "good morning", "thanks", "ok", "bye")) {
            assertTrue("expected small talk: $s", SmallTalk.matches(s))
        }
    }

    /**
     * The important direction. A real question wrongly classed as small talk
     * loses the warning that stops a false "I have never seen that".
     */
    @Test
    fun realQuestionsAreNot() {
        for (s in listOf(
            "any notes on dbms",
            "good notes on databases",
            "hey can you find my ode notes",
            "what is module 1 about",
            "thanks for the module 4 notes where is module 5",
            "test paper for AI",
        )) {
            assertFalse("expected not small talk: $s", SmallTalk.matches(s))
        }
    }

    @Test
    fun emptyIsNotSmallTalk() {
        assertFalse(SmallTalk.matches(""))
        assertFalse(SmallTalk.matches("   "))
    }
}
