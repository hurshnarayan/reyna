package app.reyna.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.attribution.Attribution

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                AskScreen()
            }
        }
    }
}

/**
 * One captured file, as the home screen shows it.
 *
 * [confidence] rather than a bare sender name is the point: the UI has to be
 * able to render "we do not know who shared this" as a first-class state, not
 * as a blank where a name should be.
 */
data class CaptureCard(
    val fileName: String,
    val senderName: String?,
    val chatName: String?,
    val whenText: String,
    val confidence: Double,
) {
    /** Delegates to the single place the naming rule lives. */
    val subtitle: String
        get() = Attribution.describe(confidence, senderName, chatName, whenText)

    /** Whether to show the "not sure who shared this" affordance. */
    val uncertain: Boolean get() = confidence < Attribution.MIN_NAMED
}

/**
 * The home screen: a question box, with recent captures beneath it.
 *
 * Deliberately not a file browser. A browser makes Reyna a worse Google Drive;
 * the product is asking for something half-remembered and getting the file
 * back with a citation.
 */
@Composable
fun AskScreen() {
    var query by remember { mutableStateOf("") }

    // Placeholder rows until the local store is wired in. They exist so the
    // three confidence bands are visible during development, because that
    // distinction is the thing most likely to be quietly lost.
    val recent = remember {
        listOf(
            CaptureCard("Compiler_Lab_Manual.pdf", "Mohit", "Sem 5 CS", "2 hours ago", 0.95),
            CaptureCard("DOC-20260818-WA0041.pdf", null, "Sem 5 CS", "18 August", 0.45),
            CaptureCard("scan_0007.jpg", null, null, "3 weeks ago", 0.0),
        )
    }

    Scaffold { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text("Ask Reyna anything", fontSize = 22.sp, fontWeight = FontWeight.Bold)
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                placeholder = { Text("that thing about the deposit") },
                modifier = Modifier.fillMaxWidth(),
            )
            Text("Recently found", fontWeight = FontWeight.SemiBold)
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(recent) { card -> CaptureRow(card) }
            }
        }
    }
}

@Composable
private fun CaptureRow(card: CaptureCard) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(card.fileName, fontWeight = FontWeight.Medium)
            Text(card.subtitle, fontSize = 13.sp)
            if (card.uncertain) {
                // Uncertainty is shown, never hidden and never rounded up into
                // a name. Tapping this is how the user repairs it.
                Text("not sure who shared this", fontSize = 12.sp)
            }
        }
    }
}
