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

    data class Answer(
        val reply: String,
        /** Filenames the backend cited, in order. */
        val files: List<CitedFile>,
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
            .addFormDataPart("user_phone", "")
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
            if (!resp.isSuccessful) error("upload ${resp.code}: ${text.take(200)}")
            val json = JSONObject(text)
            UploadResult(
                remoteId = json.optLong("file_id", 0),
                folder = json.optString("subject").ifBlank { null },
                duplicate = json.optBoolean("duplicate", false),
            )
        }
    }.onFailure { Log.w(TAG, "upload failed: ${it.message}") }

    /** Asks a question. The backend parses it, searches, and writes the reply. */
    fun ask(question: String): Result<Answer> = runCatching {
        val payload = JSONObject()
            .put("query", question)
            .toString()
            .toRequestBody(JSON)

        val req = Request.Builder().url(url("/api/nlp/retrieve")).auth().post(payload).build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("ask ${resp.code}: ${text.take(200)}")
            val json = JSONObject(text)
            Answer(
                reply = json.optString("reply"),
                files = json.optJSONArray("files").toCitedFiles(),
            )
        }
    }.onFailure { Log.w(TAG, "ask failed: ${it.message}") }

    /** Asks a question that needs reading inside the documents. */
    fun askNotes(question: String): Result<Answer> = runCatching {
        val payload = JSONObject().put("question", question).toString().toRequestBody(JSON)
        val req = Request.Builder().url(url("/api/nlp/qa")).auth().post(payload).build()
        client.newCall(req).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            if (!resp.isSuccessful) error("qa ${resp.code}: ${text.take(200)}")
            val json = JSONObject(text)
            Answer(reply = json.optString("answer"), files = emptyList())
        }
    }.onFailure { Log.w(TAG, "qa failed: ${it.message}") }

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
        private val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
