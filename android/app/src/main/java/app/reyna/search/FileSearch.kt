package app.reyna.search

import app.reyna.attribution.Attribution

/**
 * One file, as the search sees it.
 *
 * Carries a confidence rather than a rendered sender line so search and display
 * agree on what may be shown. A file whose sender is a weak guess is still
 * searchable by that guess, but the result will not print the name.
 */
data class SearchableFile(
    val id: Long,
    val fileName: String,
    val senderName: String?,
    val chatName: String?,
    val whenText: String,
    val confidence: Double,
    val isImage: Boolean = false,
)

/** A file that matched, with the spans that matched, for highlighting. */
data class FileHit(
    val file: SearchableFile,
    val score: Int,
    /** Matched indices in the filename. Empty when it matched on other fields. */
    val nameSpans: List<Int>,
)

/**
 * Searching the captured files.
 *
 * A query is matched against three things, because those are the three ways
 * people remember a file: what it was called, who sent it, and where. Someone
 * typing "mohit" wants everything Mohit shared; someone typing "sem 5" wants a
 * chat; someone typing "compiler" wants a name. One box has to serve all three,
 * so all three are searched and the strongest field wins.
 */
object FileSearch {

    /**
     * Field weights.
     *
     * The filename dominates because it is the most specific thing a user can
     * type. Sender and chat are worth less on their own, or searching for a
     * person would bury the one file whose *name* contains their name.
     */
    /** Dominates raw score, so word coverage decides the order first. */
    private const val PARTIAL_WORD_BONUS = 100_000

    private const val NAME_WEIGHT = 100
    private const val SENDER_WEIGHT = 70
    private const val CHAT_WEIGHT = 55

    /**
     * Runs a query.
     *
     * [fuzzy] false restricts matching to substrings, which is what someone
     * wants when they know exactly what they are looking for and a scored
     * subsequence would drag in noise. True allows the full subsequence match.
     * The UI exposes this as a toggle, defaulting to on, and offers it
     * explicitly when a strict search finds nothing.
     *
     * An empty query returns everything, unranked, so a blank box is the
     * unfiltered list rather than an empty screen.
     */
    fun search(
        query: String,
        files: List<SearchableFile>,
        fuzzy: Boolean = true,
    ): List<FileHit> {
        val q = query.trim()
        if (q.isEmpty()) return files.map { FileHit(it, 0, emptyList()) }

        // Every word has to land somewhere, but not all in the same place.
        //
        // Matched as one string, "scheduling notes" found nothing: the words
        // are a subsequence of no single field, and people type the filename
        // and the sender, or two words from a long name, without thinking of
        // them as one token. Each word is scored against each field, and a
        // file survives only if every word matched something.
        val words = q.split(WHITESPACE).filter { it.isNotBlank() }

        val hits = ArrayList<FileHit>()
        for (f in files) {
            var total = 0
            var spans: List<Int> = emptyList()
            var allMatched = true

            for (word in words) {
                val best = bestFieldMatch(word, f, fuzzy)
                if (best == null) {
                    allMatched = false
                    break
                }
                total += best.first
                // Highlight only what matched in the filename, since that is
                // the only field rendered on the row.
                if (best.second.isNotEmpty()) spans = spans + best.second
            }

            if (allMatched) hits += FileHit(f, total, spans.distinct().sorted())
        }

        // Requiring every word is right when it finds something and useless
        // when it does not. "scheduling notes" describes one file to a person
        // and matches none of them literally, because no filename carries both
        // words. Rather than an empty screen, fall back to files matching any
        // word, ranked so the ones matching more come first.
        if (hits.isEmpty() && words.size > 1) {
            for (f in files) {
                var total = 0
                var matched = 0
                var spans: List<Int> = emptyList()
                for (word in words) {
                    val best = bestFieldMatch(word, f, fuzzy) ?: continue
                    matched++
                    total += best.first
                    if (best.second.isNotEmpty()) spans = spans + best.second
                }
                // Scored by how many words landed first, so a file matching two
                // always outranks one matching a single word very well.
                if (matched > 0) {
                    hits += FileHit(f, matched * PARTIAL_WORD_BONUS + total, spans.distinct().sorted())
                }
            }
        }

        return hits.sortedByDescending { it.score }
    }

    /** The best score any field gives one word, with filename spans if it won there. */
    private fun bestFieldMatch(
        word: String,
        f: SearchableFile,
        fuzzy: Boolean,
    ): Pair<Int, List<Int>>? {
        var best = Int.MIN_VALUE
        var spans: List<Int> = emptyList()

        run {
            matchField(word, f.fileName, fuzzy)?.let {
                val sc = it.score + NAME_WEIGHT
                if (sc > best) { best = sc; spans = it.positions }
            }
        }
            // The sender is only searchable when we are confident enough to
            // name them. Matching on a name we would refuse to display would
            // return a file "from Mohit" that shows no sender, which reads as a
            // bug.
        if (f.confidence >= Attribution.MIN_NAMED) {
            f.senderName?.let { sender ->
                matchField(word, sender, fuzzy)?.let {
                    val sc = it.score + SENDER_WEIGHT
                    if (sc > best) { best = sc; spans = emptyList() }
                }
            }
        }
        f.chatName?.let { chat ->
            matchField(word, chat, fuzzy)?.let {
                val sc = it.score + CHAT_WEIGHT
                if (sc > best) { best = sc; spans = emptyList() }
            }
        }

        return if (best > Int.MIN_VALUE) best to spans else null
    }

    private val WHITESPACE = Regex("\\s+")

    private fun matchField(query: String, target: String, fuzzy: Boolean): FuzzySearch.Match? {
        if (!fuzzy) {
            val idx = target.lowercase().indexOf(query.lowercase())
            if (idx < 0) return null
            return FuzzySearch.Match(1000 - idx, (idx until idx + query.length).toList())
        }
        return FuzzySearch.match(query, target)
    }

    /**
     * Chat filter chips, with counts, ordered by how much each holds.
     *
     * Counts come from the current result set rather than the whole library, so
     * they say how many of *these* results are in each chat. A chip promising
     * twelve that then shows three is worse than no chip.
     */
    fun chatFacets(hits: List<FileHit>): List<Pair<String, Int>> =
        hits.mapNotNull { it.file.chatName }
            .groupingBy { it }
            .eachCount()
            .toList()
            .sortedByDescending { it.second }

    /**
     * Vocabulary for "did you mean", built from the user's own files.
     *
     * Suggesting a word Reyna does not hold would send the user to another
     * empty result, so every suggestion is guaranteed to return something.
     */
    fun vocabulary(files: List<SearchableFile>): Set<String> {
        val out = HashSet<String>()
        for (f in files) {
            out += FuzzySearch.tokenize(f.fileName)
            f.senderName?.let { if (f.confidence >= Attribution.MIN_NAMED) out += FuzzySearch.tokenize(it) }
            f.chatName?.let { out += FuzzySearch.tokenize(it) }
        }
        return out
    }
}
