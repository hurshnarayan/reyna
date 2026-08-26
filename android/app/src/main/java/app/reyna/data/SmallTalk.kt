package app.reyna.data

/**
 * Whether something someone typed is a greeting rather than a question about
 * a document.
 *
 * Used only to suppress notes that make no sense as a reply to "hi". A reply
 * to a greeting was arriving with "833 files on this phone have not been sent
 * for reading yet, so they were not searched" stapled to it, which is a
 * perfectly good warning about a search and nonsense about a hello.
 *
 * Deliberately small and exact, and it errs towards saying no. Guessing wrong
 * in this direction only means a greeting keeps an irrelevant sentence;
 * guessing wrong the other way hides a real warning from somebody who was
 * genuinely looking for a file, and that warning exists to stop a false "I
 * have never seen that", which is the worst answer this app can give.
 */
object SmallTalk {

    private val PLEASANTRIES = setOf(
        "hi", "hii", "hiii", "hello", "helo", "hey", "heyy", "yo", "hola",
        "namaste", "greetings",
        "thanks", "thank", "thanku", "thankyou", "ty", "thx",
        "ok", "okay", "k", "kk", "cool", "nice", "great", "good", "fine",
        "morning", "afternoon", "evening", "night",
        "bye", "goodbye", "cya", "sup", "test", "testing", "you", "u", "there",
    )

    /**
     * True when every word is a pleasantry and there are at most three of
     * them, so "good morning" and "hey there" count but "good notes on
     * databases" does not.
     */
    fun matches(text: String): Boolean {
        val words = Words.of(text)
        if (words.isEmpty() || words.size > 3) return false
        return words.all { it in PLEASANTRIES }
    }
}
