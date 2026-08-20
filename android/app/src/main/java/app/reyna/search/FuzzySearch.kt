package app.reyna.search

import java.util.Locale
import kotlin.math.min

/**
 * Fuzzy matching for the file list.
 *
 * Reyna's filenames are hostile to exact search. Half of them are
 * `DOC-20260818-WA0041.pdf`, and the ones people actually named are typed from
 * memory months later, misspelled, or remembered only in fragments
 * ("compiler lab" for `Compiler_Design_Lab_Manual_v2.pdf`). A substring filter
 * returns nothing for all of that and the user concludes the file is gone.
 *
 * So matching is a scored subsequence: every character of the query must appear
 * in the target in order, but not adjacently. Scoring then decides which of the
 * many things that matches is worth showing first, using the same signals fzf
 * and editor "go to file" pickers use: runs of consecutive characters are worth
 * more than scattered ones, a match at a word boundary is worth more than one
 * mid-word, and matching early beats matching late.
 *
 * Positions are returned alongside the score so the UI can highlight exactly
 * what matched, which is what makes a fuzzy result legible rather than
 * mysterious.
 */
object FuzzySearch {

    /** Where a query matched a target, and how well. */
    data class Match(
        val score: Int,
        /** Indices in the target that the query matched, ascending. */
        val positions: List<Int>,
    )

    // Scoring weights. Relative values matter, absolute ones do not.
    private const val BASE = 16          // any matched character
    private const val CONSECUTIVE = 12   // directly after the previous match
    private const val BOUNDARY = 14      // start of a word
    private const val FIRST_CHAR = 16    // start of the whole target
    private const val GAP_PENALTY = 2    // per character skipped
    private const val MAX_GAP_PENALTY = 24

    /** Bonus for an exact substring hit, which should always outrank a scatter. */
    private const val SUBSTRING_BONUS = 90
    private const val PREFIX_BONUS = 40

    /**
     * Matches [query] against [target], or returns null when the characters do
     * not appear in order.
     *
     * Case-insensitive. An empty query matches everything with score zero, so a
     * blank search box shows the unfiltered list rather than nothing.
     */
    fun match(query: String, target: String): Match? {
        if (query.isEmpty()) return Match(0, emptyList())
        if (target.isEmpty()) return null

        val q = query.lowercase(Locale.ROOT)
        val t = target.lowercase(Locale.ROOT)

        // Exact substring is the common case and deserves to win outright,
        // rather than being scored against scattered matches on equal terms.
        val idx = t.indexOf(q)
        if (idx >= 0) {
            var score = SUBSTRING_BONUS + q.length * BASE + (q.length - 1) * CONSECUTIVE
            if (idx == 0) score += PREFIX_BONUS + FIRST_CHAR
            else if (isBoundary(t, idx)) score += BOUNDARY
            // Earlier is better, but only mildly.
            score -= min(idx, 20)
            return Match(score, (idx until idx + q.length).toList())
        }

        // Subsequence walk. Greedy: takes the first available match for each
        // query character. Not optimal in theory, and indistinguishable from
        // optimal on filename-length strings.
        val positions = ArrayList<Int>(q.length)
        var ti = 0
        var score = 0
        var lastMatch = -2

        for (qc in q) {
            var found = -1
            while (ti < t.length) {
                if (t[ti] == qc) { found = ti; break }
                ti++
            }
            if (found < 0) return null

            score += BASE
            when {
                found == 0 -> score += FIRST_CHAR
                found == lastMatch + 1 -> score += CONSECUTIVE
                isBoundary(t, found) -> score += BOUNDARY
            }
            // Penalise scatter, but cap it: one large gap should not make an
            // otherwise good match unrankable.
            if (lastMatch >= 0) {
                val gap = found - lastMatch - 1
                score -= min(gap * GAP_PENALTY, MAX_GAP_PENALTY)
            }

            positions += found
            lastMatch = found
            ti = found + 1
        }

        // A short query scattered across a long name is usually noise.
        score -= min(t.length / 8, 12)
        return Match(score, positions)
    }

    /**
     * True when position [i] starts a word.
     *
     * Filenames separate words with underscores, hyphens, dots and spaces, and
     * sometimes with camelCase. Both count, so "cdl" finds
     * `Compiler_Design_Lab.pdf` and "cdl" also finds `compilerDesignLab.pdf`.
     */
    private fun isBoundary(t: String, i: Int): Boolean {
        if (i == 0) return true
        val prev = t[i - 1]
        return prev == '_' || prev == '-' || prev == '.' || prev == ' ' || prev == '/' ||
            (!prev.isLetterOrDigit()) || (prev.isDigit() != t[i].isDigit())
    }

    /**
     * Edit distance, capped, counting an adjacent transposition as one edit.
     *
     * This is Damerau-Levenshtein (optimal string alignment) rather than plain
     * Levenshtein, and the difference matters. Swapping two letters is the most
     * common typo people make, and plain Levenshtein charges two edits for it:
     * "mohti" would sit at distance 2 from "mohit" and fall outside a
     * one-edit tolerance, so the one suggestion the user obviously wanted would
     * never be offered.
     *
     * Bounded because it only ever answers "is this within a typo or two", so a
     * row whose best path already exceeds the cap is abandoned rather than
     * finished.
     */
    fun editDistance(a: String, b: String, max: Int = 3): Int {
        if (a == b) return 0
        if (kotlin.math.abs(a.length - b.length) > max) return max + 1

        val s = a.lowercase(Locale.ROOT)
        val t = b.lowercase(Locale.ROOT)

        // Three rows: the transposition case needs to look two back.
        var prev2 = IntArray(t.length + 1)
        var prev = IntArray(t.length + 1) { it }
        var curr = IntArray(t.length + 1)

        for (i in 1..s.length) {
            curr[0] = i
            var rowMin = curr[0]
            for (j in 1..t.length) {
                val cost = if (s[i - 1] == t[j - 1]) 0 else 1
                var v = minOf(
                    curr[j - 1] + 1,      // insertion
                    prev[j] + 1,          // deletion
                    prev[j - 1] + cost,   // substitution
                )
                // Adjacent transposition: "ab" against "ba", one edit.
                if (i > 1 && j > 1 && s[i - 1] == t[j - 2] && s[i - 2] == t[j - 1]) {
                    v = min(v, prev2[j - 2] + 1)
                }
                curr[j] = v
                if (v < rowMin) rowMin = v
            }
            if (rowMin > max) return max + 1
            val tmp = prev2; prev2 = prev; prev = curr; curr = tmp
        }
        return prev[t.length]
    }

    /**
     * "Did you mean" candidates for a query that found nothing.
     *
     * Drawn from words the user's own files actually contain, so a suggestion
     * is always something that will return results when tapped. Suggesting a
     * word Reyna does not hold would be worse than suggesting nothing.
     */
    fun suggestions(query: String, vocabulary: Collection<String>, max: Int = 3): List<String> {
        if (query.length < 3) return emptyList()
        val q = query.lowercase(Locale.ROOT)
        // Allow more slack in longer words, where a single typo matters less.
        val tolerance = if (q.length >= 6) 2 else 1

        return vocabulary
            .asSequence()
            .map { it.lowercase(Locale.ROOT) }
            .distinct()
            .filter { it != q && it.length >= 3 }
            .map { it to editDistance(q, it, tolerance) }
            .filter { it.second <= tolerance }
            .sortedWith(compareBy({ it.second }, { it.first.length }))
            .map { it.first }
            .take(max)
            .toList()
    }

    /**
     * Splits text into searchable words for the suggestion vocabulary.
     *
     * Filenames are delimited by underscores, hyphens and dots as often as by
     * spaces, so "Compiler_Design_Lab.pdf" has to yield four words rather than
     * one unusable token.
     */
    fun tokenize(text: String): List<String> =
        text.split(Regex("[^A-Za-z0-9]+"))
            .filter { it.length >= 3 }
            .map { it.lowercase(Locale.ROOT) }
}
