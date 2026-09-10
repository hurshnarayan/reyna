package app.reyna.search

import app.reyna.attribution.Attribution
import java.util.Locale

enum class FileKind(val displayName: String) {
    IMAGE("Images"),
    PDF("PDFs"),
    DOCUMENT("Documents"),
    SPREADSHEET("Spreadsheets"),
    PRESENTATION("Presentations"),
    ARCHIVE("Archives"),
    AUDIO("Audio"),
    OTHER("Other");

    companion object {
        fun fromExtension(ext: String): FileKind = when (ext.lowercase(Locale.ROOT)) {
            "jpg", "jpeg", "png", "webp", "heic", "gif", "bmp", "svg" -> IMAGE
            "pdf" -> PDF
            "doc", "docx", "rtf", "txt", "odt", "md" -> DOCUMENT
            "xls", "xlsx", "csv", "tsv" -> SPREADSHEET
            "ppt", "pptx" -> PRESENTATION
            "zip", "rar", "7z", "tar", "gz" -> ARCHIVE
            "mp3", "m4a", "opus", "wav", "aac", "ogg" -> AUDIO
            else -> OTHER
        }
    }
}

enum class SortMode(val label: String) {
    LATEST("Latest"),
    DATE("Date"),
    NAME("Name"),
    KIND("Kind"),
    SIZE("Size");
}

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
    val path: String = "",
    val sizeBytes: Long = 0L,
    val mtime: Long = 0L,
    val postedAt: Long = 0L,
    val isSent: Boolean = false,
    val extractedText: String? = null,
    val remoteId: Long = 0L,
) {
    val extension: String
        get() = fileName.substringAfterLast('.', "").lowercase(Locale.ROOT)

    val kind: FileKind
        get() = if (isImage) FileKind.IMAGE else FileKind.fromExtension(extension)

    val formattedSize: String
        get() = when {
            sizeBytes <= 0 -> ""
            sizeBytes < 1024L -> "$sizeBytes B"
            sizeBytes < 1024L * 1024L -> "${sizeBytes / 1024L} KB"
            sizeBytes < 1024L * 1024L * 1024L -> "%.1f MB".format(Locale.US, sizeBytes / (1024.0 * 1024.0))
            else -> "%.1f GB".format(Locale.US, sizeBytes / (1024.0 * 1024.0 * 1024.0))
        }
}

/** A file that matched, with the spans that matched, for highlighting. */
data class FileHit(
    val file: SearchableFile,
    val score: Int,
    /** Matched indices in the filename. Empty when it matched on other fields. */
    val nameSpans: List<Int>,
    val reason: MatchReason? = null,
    val snippet: String? = null,
)

enum class MatchReason(val label: String) {
    FILE_NAME("Filename match"),
    SENDER("Sender match"),
    CHAT("Chat match"),
    CONTENT("Found inside document"),
}

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
    private const val NAME_WEIGHT = 3_000
    private const val SENDER_WEIGHT = 2_000
    private const val CHAT_WEIGHT = 1_500
    private const val CONTENT_WEIGHT = 500

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
        /** IDs returned by SQLite FTS. Null keeps pure unit tests self-contained. */
        contentCandidateIds: Set<Long>? = null,
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
        val words = queryWords(q)
        if (words.isEmpty()) return emptyList()

        val hits = ArrayList<FileHit>()
        for (f in files) {
            var total = 0
            var spans: List<Int> = emptyList()
            var allMatched = true
            val reasons = ArrayList<MatchReason>()

            for (word in words) {
                val best = bestFieldMatch(word, f, fuzzy, contentCandidateIds)
                if (best == null) {
                    allMatched = false
                    break
                }
                total += best.score
                reasons += best.reason
                // Highlight only what matched in the filename, since that is
                // the only field rendered on the row.
                if (best.positions.isNotEmpty()) spans = spans + best.positions
            }

            if (allMatched) {
                val reason = strongestReason(reasons)
                hits += FileHit(
                    file = f,
                    score = total,
                    nameSpans = spans.distinct().sorted(),
                    reason = reason,
                    snippet = if (reason == MatchReason.CONTENT) contentSnippet(f.extractedText, words) else null,
                )
            }
        }

        return hits.sortedByDescending { it.score }
    }

    private data class FieldMatch(
        val score: Int,
        val positions: List<Int>,
        val reason: MatchReason,
    )

    /** The best score any field gives one word, with filename spans if it won there. */
    private fun bestFieldMatch(
        word: String,
        f: SearchableFile,
        fuzzy: Boolean,
        contentCandidateIds: Set<Long>?,
    ): FieldMatch? {
        var best = Int.MIN_VALUE
        var result: FieldMatch? = null

        run {
            matchMetadataField(word, f.fileName, fuzzy)?.let {
                val sc = it.score + NAME_WEIGHT
                if (sc > best) {
                    best = sc
                    result = FieldMatch(sc, it.positions, MatchReason.FILE_NAME)
                }
            }
        }
            // The sender is only searchable when we are confident enough to
            // name them. Matching on a name we would refuse to display would
            // return a file "from Mohit" that shows no sender, which reads as a
            // bug.
        if (f.confidence >= Attribution.MIN_NAMED) {
            f.senderName?.let { sender ->
                matchMetadataField(word, sender, fuzzy)?.let {
                    val sc = it.score + SENDER_WEIGHT
                    if (sc > best) {
                        best = sc
                        result = FieldMatch(sc, emptyList(), MatchReason.SENDER)
                    }
                }
            }
        }
        f.chatName?.let { chat ->
            matchMetadataField(word, chat, fuzzy)?.let {
                val sc = it.score + CHAT_WEIGHT
                if (sc > best) {
                    best = sc
                    result = FieldMatch(sc, emptyList(), MatchReason.CHAT)
                }
            }
        }

        // Content is never fuzzy-matched as a character subsequence. A long
        // document contains almost every short sequence by accident, which is
        // why unrelated PDFs used to flood every result. SQLite FTS narrows the
        // candidates; this exact token check then explains the match.
        val mayContain = contentCandidateIds?.contains(f.id) ?: true
        if (mayContain) {
            f.extractedText?.let { text ->
                matchField(word, text, fuzzy = false)?.let {
                    val sc = it.score + CONTENT_WEIGHT
                    if (sc > best) {
                        best = sc
                        result = FieldMatch(sc, emptyList(), MatchReason.CONTENT)
                    }
                }
            }
        }

        return result
    }

    private val TOKEN_SPLIT = Regex("[^a-zA-Z0-9]+")
    private val STOP_WORDS = setOf(
        "a", "an", "the", "file", "files", "document", "documents", "pdf", "pdfs",
        "find", "show", "search", "for", "please", "me", "my", "inside", "about",
    )

    fun queryWords(query: String): List<String> {
        val all = query.lowercase(Locale.ROOT).split(TOKEN_SPLIT).filter { it.isNotBlank() }
        val meaningful = all.filter { it !in STOP_WORDS }
        return (meaningful.ifEmpty { all }).distinct()
    }

    /** Safe FTS4 prefix query. Metadata typo matching stays outside the index. */
    fun contentIndexQuery(query: String): String =
        queryWords(query).joinToString(" OR ") { "$it*" }

    private fun matchField(query: String, target: String, fuzzy: Boolean): FuzzySearch.Match? {
        if (!fuzzy) {
            val idx = target.lowercase().indexOf(query.lowercase())
            if (idx < 0) return null
            return FuzzySearch.Match(1000 - idx, (idx until idx + query.length).toList())
        }
        return FuzzySearch.match(query, target)
    }

    /** Fuzzy filenames/people/chats, including one or two actual typo edits. */
    private fun matchMetadataField(query: String, target: String, fuzzy: Boolean): FuzzySearch.Match? {
        matchField(query, target, fuzzy)?.let { return it }
        if (!fuzzy || query.length < 3) return null

        val tolerance = if (query.length >= 7) 2 else 1
        for (token in FuzzySearch.tokenize(target)) {
            if (FuzzySearch.editDistance(query, token, tolerance) <= tolerance) {
                val start = target.indexOf(token, ignoreCase = true).coerceAtLeast(0)
                return FuzzySearch.Match(
                    score = 120 - tolerance * 20,
                    positions = (start until start + token.length).toList(),
                )
            }
        }
        return null
    }

    private fun strongestReason(reasons: List<MatchReason>): MatchReason = when {
        MatchReason.FILE_NAME in reasons -> MatchReason.FILE_NAME
        MatchReason.SENDER in reasons -> MatchReason.SENDER
        MatchReason.CHAT in reasons -> MatchReason.CHAT
        else -> MatchReason.CONTENT
    }

    private fun contentSnippet(text: String?, words: List<String>): String? {
        if (text.isNullOrBlank()) return null
        val flat = text.replace(Regex("\\s+"), " ").trim()
        val first = words.map { flat.indexOf(it, ignoreCase = true) }.filter { it >= 0 }.minOrNull() ?: return null
        val start = (first - 42).coerceAtLeast(0)
        val end = (first + 118).coerceAtMost(flat.length)
        return buildString {
            if (start > 0) append("…")
            append(flat.substring(start, end).trim())
            if (end < flat.length) append("…")
        }
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
     * Sorts files according to the chosen SortMode and direction.
     */
    fun sortFiles(
        hits: List<FileHit>,
        mode: SortMode,
        ascending: Boolean = false,
    ): List<FileHit> {
        return when (mode) {
            SortMode.LATEST -> {
                if (ascending) {
                    hits.sortedWith(compareBy<FileHit> { it.file.mtime.takeIf { t -> t > 0 } ?: it.file.postedAt }
                        .thenBy { it.file.id })
                } else {
                    hits.sortedWith(compareByDescending<FileHit> { it.file.mtime.takeIf { t -> t > 0 } ?: it.file.postedAt }
                        .thenByDescending { it.file.id })
                }
            }
            SortMode.DATE -> {
                if (ascending) {
                    hits.sortedWith(compareBy<FileHit> { it.file.postedAt }
                        .thenBy { it.file.mtime })
                } else {
                    hits.sortedWith(compareByDescending<FileHit> { it.file.postedAt }
                        .thenByDescending { it.file.mtime })
                }
            }
            SortMode.NAME -> {
                if (ascending) {
                    hits.sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.file.fileName })
                } else {
                    hits.sortedWith(compareByDescending(String.CASE_INSENSITIVE_ORDER) { it.file.fileName })
                }
            }
            SortMode.KIND -> {
                if (ascending) {
                    hits.sortedWith(compareBy<FileHit> { it.file.kind.ordinal }
                        .thenBy(String.CASE_INSENSITIVE_ORDER) { it.file.fileName })
                } else {
                    hits.sortedWith(compareBy<FileHit> { it.file.kind.ordinal }
                        .thenByDescending { it.file.mtime.takeIf { t -> t > 0 } ?: it.file.postedAt })
                }
            }
            SortMode.SIZE -> {
                if (ascending) {
                    hits.sortedWith(compareBy<FileHit> { it.file.sizeBytes }
                        .thenBy { it.file.fileName })
                } else {
                    hits.sortedWith(compareByDescending<FileHit> { it.file.sizeBytes }
                        .thenBy { it.file.fileName })
                }
            }
        }
    }

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
