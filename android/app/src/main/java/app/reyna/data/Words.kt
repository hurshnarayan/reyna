package app.reyna.data

/**
 * Whole-word matching for filenames.
 *
 * The rule this replaces was `name.contains(token)`. "ode" is inside "diodes",
 * so a question about ordinary differential equations matched a semiconductor
 * lecture and listed it as a source. "1" is inside "part1", so a question
 * about module 1 matched Module4_part1_Arrays. Both made the app look like it
 * had understood nothing, because it had not.
 *
 * The backend does the same thing in internal/relevance, and the two must
 * agree: the phone uses this to decide which unsent files to push ahead of a
 * question, and pushing the wrong ones means the server searches without the
 * document the question was about.
 */
object Words {

    /** Shortest token allowed to match by prefix. Below this a stem is a coincidence. */
    private const val MIN_STEM = 5

    /**
     * Cuts text into the words it is built from.
     *
     * Splitting at the letter/digit boundary is what separates "part1" into
     * part and 1. Without it any filename carrying a digit matches any
     * question carrying a digit.
     */
    fun of(text: String): List<String> {
        val out = ArrayList<String>()
        val cur = StringBuilder()
        var prevDigit = false
        var started = false

        fun flush() {
            if (cur.isNotEmpty()) {
                out.add(cur.toString())
                cur.setLength(0)
            }
            started = false
        }

        for (ch in text.lowercase()) {
            val letter = ch in 'a'..'z'
            val digit = ch in '0'..'9'
            if (!letter && !digit) {
                flush()
                continue
            }
            if (started && digit != prevDigit) flush()
            cur.append(ch)
            prevDigit = digit
            started = true
        }
        flush()
        return out
    }

    /** Whether [token] is one of [words], allowing singular and plural of the same word. */
    fun match(words: List<String>, token: String): Boolean {
        if (token.isEmpty()) return false
        for (w in words) {
            if (w == token) return true
            if (token.length >= MIN_STEM && w.startsWith(token)) return true
            if (w.length >= MIN_STEM && token.startsWith(w)) return true
        }
        return false
    }

    /** [match] against text that has not been split yet. */
    fun matchText(text: String, token: String): Boolean = match(of(text), token)

    /**
     * How well a filename answers a set of query tokens.
     *
     * Adjacent tokens are worth far more than scattered ones, which is what
     * separates "Module 1" from "Module4_part1": both contain the words module
     * and 1, only one of them has them side by side, and that is the file the
     * person meant.
     */
    fun score(name: String, tokens: List<String>): Int {
        if (tokens.isEmpty()) return 0
        val words = of(name)
        var total = 0
        for (t in tokens) if (match(words, t)) total += if (t[0].isDigit()) 5 else 3
        for (i in 0 until tokens.size - 1) {
            if (adjacent(words, tokens[i], tokens[i + 1])) total += 10
        }
        return total
    }

    private fun adjacent(words: List<String>, a: String, b: String): Boolean {
        for (i in 0 until words.size - 1) {
            if (match(listOf(words[i]), a) && match(listOf(words[i + 1]), b)) return true
        }
        return false
    }
}
