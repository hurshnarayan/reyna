package app.reyna.net

import android.util.Log
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.asRequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.concurrent.TimeUnit

/**
 * The Reyna backend.
 *
 * Everything here is authenticated with the shared device token; the routes it
 * calls accept file uploads and expose group contents, so they are not open.
 *
 * Failures are returned rather than thrown. Capture must never depend on the
 * network: a file that cannot be uploaded stays on the device, marked pending,
 * and goes up on the next attempt. Losing a file because the server was down
 * would defeat the point of watching the folder in the first place.
 */
class ReynaApi(
    private val baseUrl: String,
    private val deviceToken: String,
) {
    private val client = OkHttpClient.Builder()
        // Uploads are whole documents over student wifi, so the write timeout
        // has to be generous. Reads are LLM calls, which are slower still.
        .connectTimeout(15, TimeUnit.SECONDS)
        .writeTimeout(120, TimeUnit.SECONDS)
        .readTimeout(120, TimeUnit.SECONDS)
        .build()

    data class UploadResult(val remoteId: Long, val folder: String?, val duplicate: Boolean)

    /**
     * A file the server refused and will refuse again.
     *
     * Separated from a network failure because the two need opposite
     * responses. If the server cannot be reached, stopping and retrying later
     * is right. If the server looked at this file and said no, retrying is
     * pointless, and retrying it first forever is how one bad file blocks every
     * file behind it.
     */
    class Rejected(val status: Int, message: String) : Exception(message)

    data class Answer(
        val reply: String,
        /** Filenames the backend cited, in order. */
        val files: List<CitedFile>,
        /** The passages the answer rests on, already verified server side. */
        val citations: List<Citation> = emptyList(),
    )

    /**
     * One passage an answer was drawn from.
     *
     * [quote] is the line itself and [context] the lines around it, so the
     * sheet can show where the answer came from rather than asserting it did.
     */
    data class Citation(
        val fileId: Long,
        val fileName: String,
        val sender: String?,
        val sharedAt: String?,
        val folder: String?,
        val quote: String,
        val context: String,
        val confidence: Double,
        /** Page the quote sits on. Always at least 1. */
        val page: Int,
    )

    data class CitedFile(
        val name: String,
        val folder: String?,
        val sender: String?,
        val sharedAt: String?,
        val confidence: Double,
    )

    private fun url(path: String) = baseUrl.trimEnd('/') + path

    private fun Request.Builder.auth() = header("Authorization", "Bearer $deviceToken")

    fun health(): Boolean = runCatching {
        client.newCall(Request.Builder().url(url("/api/health")).build()).execute()
            .use { it.isSuccessful }
    }.getOrDefault(false)

    /**
     * Sends one captured file with everything known about who shared it.
     *
     * Attribution travels as a method and a confidence rather than a bare name,
     * so the server can apply the same rule the app does and refuse to state a
     * sender it cannot stand behind.
     */
    fun upload(
        file: File,
        fileName: String,
        mimeType: String,
        chatName: String?,
        senderName: String?,
        postedAtSeconds: Long,
        confidence: Double,
        method: String,
    ): Result<UploadResult> = runCatching {
        val body = MultipartBody.Builder().setType(MultipartBody.FORM)
            .addFormDataPart("group_wa_id", chatName ?: "device")
            .addFormDataPart("user_phone", DEVICE_IDENTITY)
            .addFormDataPart("user_name", senderName.orEmpty())
            .addFormDataPart("file_name", fileName)
            .addFormDataPart("mime_type", mimeType)
            .addFormDataPart("file_size", file.length().toString())
            .addFormDataPart("posted_at", postedAtSeconds.toString())
            .addFormDataPart("attribution_confidence", confidence.toString())
            .addFormDataPart("attribution_method", method)
            .addFormDataPart("file", fileName, file.asRequestBody(mimeType.toMediaType()))
            .build()

        val req = Request.Builder().url(url("/api/device/files")).auth().post(body).build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) {
                // 4xx is a verdict on this file. 5xx and anything else is the
                // server having a bad time, which will pass.
                if (resp.code in 400..499) {
                    throw Rejected(resp.code, "upload ${resp.code}: ${text.take(200)}")
                }
                error("upload ${resp.code}: ${text.take(200)}")
            }
            val json = JSONObject(text)
            val remoteId = json.optLong("file_id", 0)

            // A 200 that carries no file id is not a delivery. Anything can
            // return 200: a proxy, a captive portal, a stub. Treating that as
            // success marked files uploaded that the server had never seen,
            // and because upload is one-way and never re-checked, they were
            // gone for good. Fail here and the file simply stays pending.
            if (remoteId <= 0) error("upload accepted but returned no file id")

            UploadResult(
                remoteId = remoteId,
                folder = json.optString("subject").ifBlank { null },
                duplicate = json.optBoolean("duplicate", false),
            )
        }
    }.onFailure { Log.w(TAG, "upload failed: ${it.message}") }

    data class ChatContext(
        val role: String,
        val text: String,
        val fileNames: List<String> = emptyList(),
    )

    /**
     * Queries the NLP retrieve endpoint with natural language and conversation history.
     */
    fun ask(
        question: String,
        history: List<ChatContext> = emptyList(),
        onCall: (okhttp3.Call) -> Unit = {}
    ): Result<Answer> = runCatching {
        val payloadObj = JSONObject()
            .put("query", question)
            .put("user_phone", DEVICE_IDENTITY)

        if (history.isNotEmpty()) {
            val histArr = org.json.JSONArray()
            for (h in history) {
                val hObj = JSONObject()
                    .put("role", h.role)
                    .put("text", h.text)
                if (h.fileNames.isNotEmpty()) {
                    val fnArr = org.json.JSONArray()
                    h.fileNames.forEach { fnArr.put(it) }
                    hObj.put("file_names", fnArr)
                }
                histArr.put(hObj)
            }
            payloadObj.put("history", histArr)
        }

        val payload = payloadObj.toString().toRequestBody(JSON)
        val req = Request.Builder().url(url("/api/nlp/retrieve")).auth().post(payload).build()
        val call = client.newCall(req)
        onCall(call)
        call.execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("ask ${resp.code}: ${text.take(200)}")
            val json = JSONObject(text)
            Answer(
                reply = json.optString("reply"),
                files = json.optJSONArray("files").toCitedFiles(),
                citations = json.optJSONArray("citations").toCitations(),
            )
        }
    }.onFailure { Log.w(TAG, "ask failed: ${it.message}") }

    /** Asks a question that needs reading inside the documents. */
    fun askNotes(question: String): Result<Answer> = runCatching {
        val payload = JSONObject()
            .put("question", question)
            .put("user_phone", DEVICE_IDENTITY)
            .toString().toRequestBody(JSON)
        val req = Request.Builder().url(url("/api/nlp/qa")).auth().post(payload).build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("qa ${resp.code}: ${text.take(200)}")
            val json = JSONObject(text)
            Answer(reply = json.optString("answer"), files = emptyList())
        }
    }.onFailure { Log.w(TAG, "qa failed: ${it.message}") }

    /** One message the phone learned about, for /api/device/events. */
    data class DeviceEvent(
        val chatKey: String,
        val chatName: String,
        val senderName: String,
        val postedAtSeconds: Long,
        val text: String,
        val attachmentName: String,
        val source: String,
    )

    /**
     * Sends observed messages so the server can re-run the join.
     *
     * Batched because the phone accumulates events while offline, and one
     * request per notification would be slow and a good way to get rate
     * limited. Duplicates collapse server-side, so resending after a failure
     * costs nothing.
     */
    fun sendEvents(chatName: String?, events: List<DeviceEvent>): Result<Int> = runCatching {
        val arr = JSONArray()
        for (e in events) {
            arr.put(
                JSONObject()
                    .put("chat_key", e.chatKey)
                    .put("chat_name", e.chatName)
                    .put("sender_name", e.senderName)
                    .put("posted_at", e.postedAtSeconds)
                    .put("text", e.text)
                    .put("attachment_name", e.attachmentName)
                    .put("has_attachment", e.attachmentName.isNotEmpty())
                    .put("source", e.source)
            )
        }
        val payload = JSONObject()
            .put("group_wa_id", chatName ?: "device")
            .put("events", arr)
            .toString().toRequestBody(JSON)
        val req = Request.Builder().url(url("/api/device/events")).auth().post(payload).build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("events ${resp.code}")
            JSONObject(text).optInt("stored", 0)
        }
    }.onFailure { Log.w(TAG, "sendEvents failed: ${it.message}") }

    /**
     * Sends attachment rows parsed from a chat export.
     *
     * Only the rows, never the export text: the raw chat is parsed on the
     * phone and discarded there, which is what makes the import promise
     * checkable rather than a claim.
     */
    fun sendExport(
        chatName: String?,
        events: List<DeviceEvent>,
        dateOrderProven: Boolean,
    ): Result<Int> = runCatching {
        val arr = JSONArray()
        for (e in events) {
            arr.put(
                JSONObject()
                    .put("chat_key", e.chatKey)
                    .put("chat_name", e.chatName)
                    .put("sender_name", e.senderName)
                    .put("posted_at", e.postedAtSeconds)
                    .put("attachment_name", e.attachmentName)
                    .put("has_attachment", true)
                    .put("source", "export")
            )
        }
        val payload = JSONObject()
            .put("group_wa_id", chatName ?: "device")
            .put("chat_name", chatName.orEmpty())
            .put("events", arr)
            .put("date_order_proven", dateOrderProven)
            .toString().toRequestBody(JSON)
        val req = Request.Builder().url(url("/api/device/export")).auth().post(payload).build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("export ${resp.code}")
            JSONObject(text).optInt("improved", 0)
        }
    }.onFailure { Log.w(TAG, "sendExport failed: ${it.message}") }

    /** Tells the server the user corrected an attribution. */
    fun setSender(fileId: Long, sender: String): Result<Unit> = runCatching {
        val payload = JSONObject().put("file_id", fileId).put("sender", sender)
            .toString().toRequestBody(JSON)
        val req = Request.Builder().url(url("/api/device/attribute")).auth().post(payload).build()
        client.newCall(req).execute().use { if (!it.isSuccessful) error("attribute ${it.code}") }
    }.onFailure { Log.w(TAG, "setSender failed: ${it.message}") }

    /**
     * Asks the server for a Google OAuth URL.
     *
     * The app cannot complete OAuth itself. The client secret lives on the
     * server, and shipping it inside an APK would hand it to every user, so the
     * app opens this URL in a browser and the callback lands back on the
     * server, which stores the tokens.
     */
    fun driveConnectUrl(): Result<String?> = runCatching {
        val req = Request.Builder().url(url("/api/device/drive/connect?phone=device")).auth().build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("drive connect ${resp.code}")
            val json = JSONObject(text)
            if (!json.optBoolean("configured", false)) null
            else json.optString("url").ifBlank { null }
        }
    }.onFailure { Log.w(TAG, "driveConnectUrl failed: ${it.message}") }

    fun driveConnected(): Boolean = runCatching {
        val req = Request.Builder().url(url("/api/device/drive/status?phone=device")).auth().build()
        client.newCall(req).execute().use { resp ->
            resp.isSuccessful && JSONObject(resp.body?.string().orEmpty()).optBoolean("connected", false)
        }
    }.getOrDefault(false)


    /** What is waiting to reach Drive, and what already got there. */
    data class DriveState(
        val connected: Boolean,
        val email: String?,
        val pending: Int,
        val inDrive: Int,
        val examples: List<String>,
    )

    /**
     * Asks the server what has actually reached Drive.
     *
     * The phone knows a file left the device; it cannot know whether the server
     * ever filed it, because upload and commit are separate and commit runs on
     * a timer. Without asking, the app can only imply everything is safe, which
     * is the one thing it should not imply on someone's behalf.
     */
    fun driveState(): Result<DriveState> = runCatching {
        val req = Request.Builder().url(url("/api/device/drive/state?phone=$DEVICE_IDENTITY")).auth().build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("drive state ${resp.code}")
            val json = JSONObject(text)
            val arr = json.optJSONArray("examples")
            DriveState(
                connected = json.optBoolean("connected", false),
                email = json.optString("email").ifBlank { null },
                pending = json.optInt("pending", 0),
                inDrive = json.optInt("in_drive", 0),
                examples = (0 until (arr?.length() ?: 0)).map { arr!!.optString(it) },
            )
        }
    }.onFailure { Log.w(TAG, "driveState failed: ${it.message}") }

    /** Forgets the Google tokens. Nothing already in Drive is touched. */
    fun driveDisconnect(): Result<Unit> = runCatching {
        val req = Request.Builder()
            .url(url("/api/device/drive/disconnect?phone=$DEVICE_IDENTITY"))
            .auth()
            .post("{}".toRequestBody(JSON))
            .build()
        client.newCall(req).execute().use { if (!it.isSuccessful) error("disconnect ${it.code}") }
    }.onFailure { Log.w(TAG, "driveDisconnect failed: ${it.message}") }

    /** Files everything staged into Drive now instead of waiting for the timer. */
    fun drivePush(): Result<Int> = runCatching {
        val req = Request.Builder()
            .url(url("/api/device/drive/push?phone=$DEVICE_IDENTITY"))
            .auth()
            .post("{}".toRequestBody(JSON))
            .build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("drive push ${resp.code}")
            JSONObject(text).optInt("uploaded", 0)
        }
    }.onFailure { Log.w(TAG, "drivePush failed: ${it.message}") }

    private fun JSONArray?.toCitations(): List<Citation> {
        if (this == null) return emptyList()
        return (0 until length()).mapNotNull { i ->
            val o = optJSONObject(i) ?: return@mapNotNull null
            val quote = o.optString("quote")
            if (quote.isBlank()) return@mapNotNull null
            Citation(
                fileId = o.optLong("file_id", 0),
                fileName = o.optString("file_name"),
                sender = o.optString("sender").ifBlank { null },
                sharedAt = o.optString("shared_at").ifBlank { null },
                folder = o.optString("folder").ifBlank { null },
                quote = quote,
                context = o.optString("context").ifBlank { quote },
                confidence = o.optDouble("attribution_confidence", 0.0),
                page = o.optInt("page", 1).coerceAtLeast(1),
            )
        }
    }

    private fun JSONArray?.toCitedFiles(): List<CitedFile> {
        if (this == null) return emptyList()
        return (0 until length()).mapNotNull { i ->
            val o = optJSONObject(i) ?: return@mapNotNull null
            CitedFile(
                name = o.optString("file_name"),
                folder = o.optString("subject").ifBlank { null },
                sender = o.optString("shared_by_name").ifBlank { null },
                sharedAt = o.optString("posted_at").ifBlank { null },
                confidence = o.optDouble("attribution_confidence", 0.0),
            )
        }
    }

    companion object {
        private const val TAG = "ReynaApi"

        /**
         * Who the phone says it is.
         *
         * The backend was built around group members identified by phone
         * number, which an on-device capture does not have and should not ask
         * for. One stable identity keeps uploads, questions and Drive on the
         * same user record; the device endpoints already default to this same
         * value, so it is the convention rather than a new one.
         */
        const val DEVICE_IDENTITY = "device"
        private val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
