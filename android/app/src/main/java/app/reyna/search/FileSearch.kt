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

        val hits = ArrayList<FileHit>()
        for (f in files) {
            var best = Int.MIN_VALUE
            var spans: List<Int> = emptyList()

            matchField(q, f.fileName, fuzzy)?.let {
                val s = it.score + NAME_WEIGHT
                if (s > best) { best = s; spans = it.positions }
            }
            // The sender is only searchable when we are confident enough to
            // name them. Matching on a name we would refuse to display would
            // return a file "from Mohit" that shows no sender, which reads as a
            // bug.
            if (f.confidence >= Attribution.MIN_NAMED) {
                f.senderName?.let { sender ->
                    matchField(q, sender, fuzzy)?.let {
                        val s = it.score + SENDER_WEIGHT
                        if (s > best) { best = s; spans = emptyList() }
                    }
                }
            }
            f.chatName?.let { chat ->
                matchField(q, chat, fuzzy)?.let {
                    val s = it.score + CHAT_WEIGHT
                    if (s > best) { best = s; spans = emptyList() }
                }
            }

            if (best > Int.MIN_VALUE) hits += FileHit(f, best, spans)
        }

        return hits.sortedByDescending { it.score }
    }

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
