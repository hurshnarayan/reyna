package app.reyna.data

import android.content.Context
import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.Index
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Room
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase
import androidx.room.RoomDatabase
import kotlinx.coroutines.flow.Flow

/**
 * A file Reyna found on disk.
 *
 * [sha256] is the identity, not the path: the observer and the reconcile scan
 * both see the same file, and the same document forwarded twice is one
 * document. [postedAt] is when the message was sent and [mtime] is when the
 * file reached the disk, and they are separate columns because on-device they
 * can differ by days.
 */
@Entity(tableName = "files", indices = [Index(value = ["sha256"], unique = true), Index("postedAt")])
data class FileEntity(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val path: String,
    val name: String,
    val sha256: String,
    val sizeBytes: Long,
    /** When the file landed on disk. */
    val mtime: Long,
    /** When it was posted, once known. Falls back to mtime. */
    val postedAt: Long,
    val isImage: Boolean,
    /** True when found under WhatsApp's /Sent/ folder. */
    val isSent: Boolean,

    val senderName: String? = null,
    val chatName: String? = null,
    val confidence: Double = 0.0,
    val method: String = "",

    /** Set once the backend has accepted it. */
    val uploaded: Boolean = false,
    val remoteId: Long = 0,
    val folder: String? = null,
)

/**
 * A message we learned about, from a notification or a chat export.
 *
 * Stored whether or not a file ever turns up for it: a file downloaded hours
 * later still needs this row to exist, which is why the join runs in both
 * directions.
 */
@Entity(
    tableName = "events",
    indices = [
        Index("postedAt"),
        Index(value = ["chatKey", "postedAt", "senderDisplay", "attachmentName"], unique = true),
    ],
)
data class EventEntity(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val chatKey: String,
    val chatName: String,
    val senderKey: String,
    val senderDisplay: String,
    val postedAt: Long,
    val text: String,
    val attachmentName: String,
    val hasAttachment: Boolean,
    /** notification | notification_fallback | export */
    val source: String,
)

/**
 * A candidate file-to-event match.
 *
 * Every candidate is kept rather than only the winner, so a chat export
 * imported later can promote a better match without the earlier reasoning being
 * lost, and a wrong guess can be explained after the fact.
 */
@Entity(tableName = "links", primaryKeys = ["fileId", "eventId"], indices = [Index("fileId")])
data class LinkEntity(
    val fileId: Long,
    val eventId: Long,
    val method: String,
    val confidence: Double,
    val isActive: Boolean,
    val linkedAt: Long,
)

/** One turn of the conversation, so it survives the app being killed. */
@Entity(tableName = "messages")
data class MessageEntity(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    val text: String,
    val fromUser: Boolean,
    val at: Long,
    /** File ids this answer attached, comma separated. */
    val fileIds: String = "",

    /**
     * The passages this answer rests on, as JSON.
     *
     * Stored with the message rather than rebuilt from the files, unlike the
     * chips. A chip should always show current attribution, but a citation is
     * a record of what the answer was based on at the time it was given, and
     * rewriting history under an old answer would make the evidence useless.
     */
    val citations: String = "",

    /**
     * Marks a reply that is about Reyna's own state rather than about the
     * user's documents, so it can be shown as a notice instead of an answer.
     * Empty for ordinary messages.
     */
    val notice: String = "",
)

@Dao
interface ReynaDao {

    // ── Files ──

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertFile(file: FileEntity): Long

    @Query("SELECT * FROM files ORDER BY postedAt DESC")
    fun observeFiles(): Flow<List<FileEntity>>

    @Query("SELECT * FROM files ORDER BY postedAt DESC")
    suspend fun allFiles(): List<FileEntity>

    @Query("SELECT * FROM files WHERE id = :id")
    suspend fun file(id: Long): FileEntity?

    /**
     * Path plus mtime, so the reconcile scan can skip known files without
     * hashing them. Hashing every file every fifteen minutes would be the
     * difference between a background job nobody notices and one that drains
     * the battery.
     */
    @Query("SELECT path || '|' || mtime FROM files")
    suspend fun knownPathKeys(): List<String>

    @Query("SELECT COUNT(*) FROM files WHERE sha256 = :hash")
    suspend fun countByHash(hash: String): Int

    /**
     * The file already held under this hash, if any.
     *
     * Returns the row rather than a count so a duplicate can be reported by
     * name and linked to, instead of only denied.
     */
    @Query("SELECT * FROM files WHERE sha256 = :hash LIMIT 1")
    suspend fun fileByHash(hash: String): FileEntity?

    @Query("SELECT * FROM files WHERE confidence < :threshold ORDER BY postedAt DESC")
    suspend fun unattributed(threshold: Double): List<FileEntity>

    @Query("UPDATE files SET senderName = :sender, chatName = :chat, confidence = :confidence, method = :method, postedAt = :postedAt WHERE id = :id")
    suspend fun setAttribution(id: Long, sender: String?, chat: String?, confidence: Double, method: String, postedAt: Long)

    @Query("UPDATE files SET uploaded = 1, remoteId = :remoteId, folder = :folder WHERE id = :id")
    suspend fun markUploaded(id: Long, remoteId: Long, folder: String?)

    @Query("SELECT * FROM files WHERE uploaded = 0 ORDER BY postedAt ASC LIMIT :limit")
    suspend fun pendingUpload(limit: Int): List<FileEntity>

    @Query("SELECT COUNT(*) FROM files")
    fun observeFileCount(): Flow<Int>

    @Query("DELETE FROM files")
    suspend fun clearFiles()

    // ── Events ──

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertEvent(event: EventEntity): Long

    @Query("SELECT * FROM events WHERE postedAt BETWEEN :from AND :to ORDER BY postedAt DESC")
    suspend fun eventsBetween(from: Long, to: Long): List<EventEntity>

    @Query("SELECT * FROM events ORDER BY postedAt DESC LIMIT :limit")
    suspend fun recentEvents(limit: Int): List<EventEntity>

    @Query("SELECT DISTINCT chatName FROM events WHERE chatName != ''")
    suspend fun knownChats(): List<String>

    @Query("SELECT COUNT(*) FROM events")
    suspend fun eventCount(): Int

    @Query("DELETE FROM events")
    suspend fun clearEvents()

    // ── Links ──

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun insertLink(link: LinkEntity)

    @Query("UPDATE links SET isActive = 0 WHERE fileId = :fileId")
    suspend fun deactivateLinks(fileId: Long)

    @Query("DELETE FROM links")
    suspend fun clearLinks()

    // ── Messages ──

    @Insert
    suspend fun insertMessage(m: MessageEntity): Long

    @Query("SELECT * FROM messages ORDER BY at ASC")
    fun observeMessages(): Flow<List<MessageEntity>>

    @Query("SELECT * FROM messages ORDER BY at DESC LIMIT :limit")
    suspend fun recentMessages(limit: Int): List<MessageEntity>

    @Query("SELECT COUNT(*) FROM messages")
    suspend fun messageCount(): Int

    @Query("DELETE FROM messages")
    suspend fun clearMessages()

    /**
     * Drops one answer and anything after it, so a question can be asked
     * again.
     *
     * By timestamp rather than id, because that is the order the conversation
     * is read in and the order the history is built from. Nothing is written
     * back over the old answer: a reply that has been replaced should leave no
     * trace, or the next question carries a turn the user rejected.
     */
    @Query("DELETE FROM messages WHERE at >= :from")
    suspend fun deleteMessagesFrom(from: Long)

    @Query("SELECT * FROM messages WHERE at < :before AND fromUser = 1 ORDER BY at DESC LIMIT 1")
    suspend fun lastQuestionBefore(before: Long): MessageEntity?
}

@Database(
    entities = [FileEntity::class, EventEntity::class, LinkEntity::class, MessageEntity::class],
    version = 3,
    exportSchema = false,
)
abstract class ReynaDb : RoomDatabase() {
    abstract fun dao(): ReynaDao

    companion object {
        @Volatile private var instance: ReynaDb? = null

        /**
         * Adds the notice column to messages.
         *
         * Written out rather than left to the destructive fallback, which
         * drops every table. This database is the phone's index of every
         * captured file and the attribution behind each one, rebuilt only by a
         * full rescan and, for anything learned from a notification, not
         * rebuildable at all. Losing a conversation to a schema change would be
         * a nuisance; losing that is the app.
         */
        private val MIGRATION_2_3 = object : Migration(2, 3) {
            override fun migrate(db: SupportSQLiteDatabase) {
                db.execSQL("ALTER TABLE messages ADD COLUMN notice TEXT NOT NULL DEFAULT ''")
            }
        }

        fun get(context: Context): ReynaDb = instance ?: synchronized(this) {
            instance ?: Room.databaseBuilder(
                context.applicationContext,
                ReynaDb::class.java,
                "reyna.db",
            )
                .addMigrations(MIGRATION_2_3)
                // Still the last resort for a mismatch nothing above covers,
                // but every schema change from here needs its own migration
                // above or it silently wipes the library.
                .fallbackToDestructiveMigration()
                .build().also { instance = it }
        }
    }
}
