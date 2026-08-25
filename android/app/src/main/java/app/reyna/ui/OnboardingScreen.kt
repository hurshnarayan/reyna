package app.reyna.ui

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ArrowBack
import androidx.compose.material.icons.rounded.BatteryFull
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.CloudUpload
import androidx.compose.material.icons.rounded.FolderOpen
import androidx.compose.material.icons.rounded.NotificationsActive
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.R
import app.reyna.permissions.Permissions
import app.reyna.ui.theme.Dimens
import app.reyna.ui.components.ReynaMarkSplash
import app.reyna.ui.theme.reynaColors

/**
 * First run.
 *
 * One thing per screen, a progress bar, and a Continue that is disabled until
 * the step is actually satisfied, rather than a button that advances past a
 * permission the user never granted.
 *
 * The order is deliberate and not the obvious one. Notifications come before
 * storage because **notification history cannot be backfilled**: a listener
 * only sees what is posted after it is enabled, so every hour without it is
 * attribution lost permanently. Files, by contrast, are already sitting on disk
 * and can be picked up whenever access is granted.
 */
enum class OnboardingStep {
    Welcome,
    Notifications,
    Storage,
    FirstScan,
    Battery,
    Drive,
    ;

    val index: Int get() = ordinal
    companion object { val total = entries.size }
}

@Composable
fun OnboardingScreen(
    step: OnboardingStep,
    permissions: List<Permissions.State>,
    scannedCount: Int,
    scanning: Boolean,
    onBack: () -> Unit,
    onGrant: (Permissions.Kind) -> Unit,
    onContinue: () -> Unit,
    onSkip: () -> Unit,
    driveConnect: DriveConnectState = DriveConnectState.Idle,
    onConnectDrive: () -> Unit = {},
) {
    val c = reynaColors
    val granted = permissions.associate { it.kind to it.granted }

    // Continue is gated on the real system state, not on having visited the
    // screen. A step that cannot be satisfied is skippable instead.
    val canContinue = when (step) {
        OnboardingStep.Welcome -> true
        OnboardingStep.Notifications -> granted[Permissions.Kind.Notifications] == true
        OnboardingStep.Storage -> granted[Permissions.Kind.Storage] == true
        // Never gated on the scan. It runs in the background and keeps running
        // after onboarding ends, so making someone wait for it buys nothing and
        // is the most likely place to lose them: a first run on a full phone is
        // the slowest this screen will ever be, and it is also the moment they
        // have the least invested.
        OnboardingStep.FirstScan -> true
        OnboardingStep.Battery -> true
        OnboardingStep.Drive -> true
    }
    val skippable = step == OnboardingStep.Notifications ||
        step == OnboardingStep.Battery ||
        step == OnboardingStep.Drive

    Column(
        Modifier
            .fillMaxSize()
            .background(c.background)
            .statusBarsPadding()
            .padding(horizontal = 20.dp),
    ) {
        if (step != OnboardingStep.Welcome) {
            Row(
                Modifier.fillMaxWidth().padding(vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    Modifier.size(40.dp).clip(CircleShape).background(c.bubbleIncoming)
                        .clickable { onBack() },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(Icons.Rounded.ArrowBack, "Back", tint = c.onSurface, modifier = Modifier.size(20.dp))
                }
                Spacer(Modifier.size(14.dp))
                ProgressBar(step.index.toFloat() / (OnboardingStep.total - 1))
            }
        }

        // BoxWithConstraints, so a step that wants to be centred has a height
        // to be centred in.
        //
        // The content scrolls, because a permission step on a small screen in
        // a large font does not fit. Inside a scroll a child asking to fill
        // the height gets the height of its own content instead of the
        // viewport, so the welcome screen's centring silently did nothing and
        // the mark sat jammed against the top of the page with the rest of the
        // screen empty below it. Handing the step a minimum height fixes the
        // centring without taking the scrolling away.
        BoxWithConstraints(Modifier.weight(1f)) {
            // Measured outside the scroll, not inside it.
            //
            // A scrolling container hands its child an unbounded height, so
            // asking for the viewport from within one returns infinity and any
            // minimum built from it means nothing. The constraints are read
            // here, where weight(1f) has already fixed the height, and the
            // scrolling is applied to the content underneath.
            val viewport = maxHeight
            Box(Modifier.verticalScroll(rememberScrollState())) {
            when (step) {
                OnboardingStep.Welcome -> Welcome(Modifier.heightIn(min = viewport))
                OnboardingStep.Notifications -> PermissionStep(
                    icon = Icons.Rounded.NotificationsActive,
                    title = "Let Reyna see who shared a file",
                    body = "A file saved to your phone carries no sender. WhatsApp's notification does. " +
                        "Reyna reads only WhatsApp notifications, takes only the name, the chat and the time, " +
                        "and none of it leaves this phone.",
                    // Stated because it is the reason this step is first, and
                    // the user is entitled to know the cost of skipping it.
                    warning = "Reyna cannot learn this later. Anything shared before you turn it on will need a chat import instead.",
                    granted = granted[Permissions.Kind.Notifications] == true,
                    onGrant = { onGrant(Permissions.Kind.Notifications) },
                )
                OnboardingStep.Storage -> PermissionStep(
                    icon = Icons.Rounded.FolderOpen,
                    title = "Let Reyna read the files",
                    body = "WhatsApp already saves the files you download to this folder:\n\n" +
                        "Android/media/com.whatsapp/WhatsApp/Media\n\n" +
                        "Reyna only reads it. It never writes there, and it looks nowhere else.",
                    granted = granted[Permissions.Kind.Storage] == true,
                    onGrant = { onGrant(Permissions.Kind.Storage) },
                )
                OnboardingStep.FirstScan -> FirstScan(scannedCount, scanning)
                OnboardingStep.Battery -> PermissionStep(
                    icon = Icons.Rounded.BatteryFull,
                    title = "Keep Reyna running",
                    body = "Android stops background apps to save power. Without an exemption, " +
                        "Reyna stops watching while your phone is idle and misses files shared overnight.",
                    granted = granted[Permissions.Kind.Battery] == true,
                    onGrant = { onGrant(Permissions.Kind.Battery) },
                )
                OnboardingStep.Drive -> DriveStep(driveConnect, onConnectDrive)
            }
            }
        }

        Column(Modifier.navigationBarsPadding().padding(bottom = 16.dp)) {
            PrimaryButton(
                text = when (step) {
                    OnboardingStep.Welcome -> "Get started"
                    OnboardingStep.Drive -> "Finish"
                    else -> "Continue"
                },
                enabled = canContinue,
                onClick = onContinue,
            )
            val connected = driveConnect is DriveConnectState.Connected
            if ((skippable && !canContinue || step == OnboardingStep.Battery) ||
                (step == OnboardingStep.Drive && !connected)
            ) {
                Spacer(Modifier.height(10.dp))
                Text(
                    "Skip for now",
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                    color = c.onSurfaceMuted,
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onSkip() }
                        .padding(vertical = 6.dp),
                    textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                )
            }
        }
    }
}

@Composable
private fun ProgressBar(fraction: Float) {
    val c = reynaColors
    val animated by animateFloatAsState(fraction, label = "onboarding-progress")
    Box(
        Modifier.fillMaxWidth().height(5.dp).clip(CircleShape).background(c.bubbleIncoming),
    ) {
        Box(
            Modifier
                .fillMaxWidth(animated.coerceIn(0f, 1f))
                .height(5.dp)
                .clip(CircleShape)
                .background(c.onSurface)
        )
    }
}

@Composable
private fun Welcome(modifier: Modifier = Modifier) {
    val c = reynaColors
    Column(
        modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        // The mark assembles itself here, which is the one place in the app
        // with the room and the attention for it. No disc behind it: the logo
        // is the three drops, and a filled circle is a container it does not
        // need.
        ReynaMarkSplash(
            color = c.onSurface,
            modifier = Modifier.size(104.dp),
        )
        Spacer(Modifier.height(30.dp))
        Text(
            "Your chats already have\nwhat you need",
            fontSize = 27.sp,
            fontWeight = FontWeight.Bold,
            color = c.onSurface,
            lineHeight = 34.sp,
            textAlign = androidx.compose.ui.text.style.TextAlign.Center,
        )
        Spacer(Modifier.height(12.dp))
        Text(
            "Reyna reads the files your chats already saved to this phone, and remembers who shared them. Invoices, tickets, contracts, notes, anything.",
            fontSize = 15.sp,
            color = c.onSurfaceMuted,
            lineHeight = 22.sp,
            textAlign = androidx.compose.ui.text.style.TextAlign.Center,
        )
        Spacer(Modifier.height(20.dp))
        Text(
            "No bot joins your groups. Nothing to ban.",
            fontSize = 13.sp,
            color = c.confident,
            fontWeight = FontWeight.Medium,
        )
    }
}

@Composable
private fun PermissionStep(
    icon: ImageVector,
    title: String,
    body: String,
    granted: Boolean,
    onGrant: () -> Unit,
    warning: String? = null,
) {
    val c = reynaColors
    Column(Modifier.fillMaxWidth().padding(top = 12.dp)) {
        Box(
            Modifier.size(52.dp).clip(RoundedCornerShape(16.dp)).background(c.bubbleIncoming),
            contentAlignment = Alignment.Center,
        ) {
            Icon(icon, null, tint = c.onSurface, modifier = Modifier.size(26.dp))
        }
        Spacer(Modifier.height(18.dp))
        Text(title, fontSize = 25.sp, fontWeight = FontWeight.Bold, color = c.onSurface, lineHeight = 31.sp)
        Spacer(Modifier.height(12.dp))
        Text(body, fontSize = 15.sp, color = c.onSurfaceMuted, lineHeight = 22.sp)

        if (warning != null && !granted) {
            Spacer(Modifier.height(14.dp))
            Box(
                Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(c.partial.copy(alpha = 0.10f))
                    .padding(12.dp),
            ) {
                Text(warning, fontSize = 13.sp, color = c.partial, lineHeight = 19.sp)
            }
        }

        Spacer(Modifier.height(22.dp))
        if (granted) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Rounded.CheckCircle, null, tint = c.confident, modifier = Modifier.size(20.dp))
                Spacer(Modifier.size(8.dp))
                Text("Granted", fontSize = 15.sp, fontWeight = FontWeight.Medium, color = c.confident)
            }
        } else {
            SecondaryButton("Open settings", onGrant)
        }
    }
}

/**
 * Results, before asking for anything else.
 *
 * The user sees their own files here, which is the payoff for the two
 * permissions they just granted and the reason the remaining optional steps get
 * a fair hearing.
 */
@Composable
private fun FirstScan(count: Int, scanning: Boolean) {
    val c = reynaColors
    Column(
        Modifier.fillMaxSize(),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        if (scanning) {
            // The count is shown while scanning too. A number going up is proof
            // of work; a sentence promising it takes a moment is not, and after
            // fifteen seconds it reads as a hang.
            Text("$count", fontSize = 56.sp, fontWeight = FontWeight.Bold, color = c.onSurface)
            Spacer(Modifier.height(4.dp))
            Text("files so far", fontSize = 16.sp, color = c.onSurfaceMuted)
            Spacer(Modifier.height(20.dp))
            Text(
                "Still looking. You can carry on, this keeps running.",
                fontSize = 15.sp,
                color = c.onSurfaceMuted,
                textAlign = androidx.compose.ui.text.style.TextAlign.Center,
            )
        } else {
            Text("$count", fontSize = 56.sp, fontWeight = FontWeight.Bold, color = c.onSurface)
            Spacer(Modifier.height(4.dp))
            Text(
                if (count == 1) "file found" else "files found",
                fontSize = 16.sp, color = c.onSurfaceMuted,
            )
            Spacer(Modifier.height(20.dp))
            Text(
                if (count == 0)
                    "Nothing yet. Reyna will pick up files as they arrive, and a chat import can bring in the older ones."
                else
                    "Reyna will keep watching. New files appear here on their own.",
                fontSize = 15.sp,
                color = c.onSurfaceMuted,
                lineHeight = 22.sp,
                textAlign = androidx.compose.ui.text.style.TextAlign.Center,
            )
        }
    }
}

/**
 * Connecting Drive, with the outcome on the screen.
 *
 * The consent screen runs in a browser and finishes on the server, so the app
 * only learns the result when it is resumed. Until it does, the button has to
 * say it is waiting; once it does, it has to say which way it went. Reporting
 * only through a snackbar left every outcome looking the same as no outcome,
 * which is how a working button got reported as dead.
 */
@Composable
private fun DriveStep(state: DriveConnectState, onConnectDrive: () -> Unit) {
    val c = reynaColors
    Column(Modifier.fillMaxWidth().padding(top = 12.dp)) {
        Box(
            Modifier.size(52.dp).clip(RoundedCornerShape(16.dp)).background(c.bubbleIncoming),
            contentAlignment = Alignment.Center,
        ) {
            Icon(Icons.Rounded.CloudUpload, null, tint = c.onSurface, modifier = Modifier.size(26.dp))
        }
        Spacer(Modifier.height(18.dp))
        Text("Where your files go", fontSize = 25.sp, fontWeight = FontWeight.Bold, color = c.onSurface)
        Spacer(Modifier.height(12.dp))
        Text(
            "Connect Google Drive and Reyna files everything into folders in your own Drive, sorted by topic.\n\n" +
                "Everything works without it. Files stay on this phone and are still searchable.",
            fontSize = 15.sp, color = c.onSurfaceMuted, lineHeight = 22.sp,
        )

        if (state is DriveConnectState.Failed) {
            Spacer(Modifier.height(18.dp))
            Box(
                Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(c.partial.copy(alpha = 0.10f))
                    .padding(12.dp),
            ) {
                Text(state.reason, fontSize = 13.sp, color = c.partial, lineHeight = 19.sp)
            }
        }

        Spacer(Modifier.height(22.dp))
        when (state) {
            is DriveConnectState.Connected -> Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Rounded.CheckCircle, null, tint = c.confident, modifier = Modifier.size(20.dp))
                Spacer(Modifier.size(8.dp))
                Text(
                    if (state.email.isBlank()) "Connected" else "Connected as ${state.email}",
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Medium,
                    color = c.confident,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            // Inert on purpose while the browser is open. Tapping again would
            // mint a second consent URL and leave two half-finished flows.
            DriveConnectState.Connecting -> BusyButton("Waiting for Google")
            is DriveConnectState.Failed -> SecondaryButton("Try again", onConnectDrive)
            DriveConnectState.Idle -> SecondaryButton("Connect Drive", onConnectDrive)
        }
    }
}

/** A button that is visibly working and deliberately does nothing when tapped. */
@Composable
private fun BusyButton(text: String) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .clip(CircleShape)
            .border(1.5.dp, c.onSurfaceMuted, CircleShape)
            .padding(vertical = 14.dp),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        CircularProgressIndicator(
            Modifier.size(16.dp),
            color = c.onSurfaceMuted,
            strokeWidth = 2.dp,
        )
        Spacer(Modifier.size(10.dp))
        Text(text, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = c.onSurfaceMuted)
    }
}

@Composable
fun PrimaryButton(text: String, enabled: Boolean, onClick: () -> Unit) {
    val c = reynaColors
    Box(
        Modifier
            .fillMaxWidth()
            .clip(CircleShape)
            // Disabled is visibly disabled rather than merely inert, so a
            // blocked step reads as blocked instead of broken.
            .background(if (enabled) c.onSurface else c.bubbleIncoming)
            .clickable(enabled = enabled) { onClick() }
            .padding(vertical = 16.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text,
            fontSize = 16.sp,
            fontWeight = FontWeight.SemiBold,
            color = if (enabled) c.background else c.onSurfaceMuted,
        )
    }
}

@Composable
private fun SecondaryButton(text: String, onClick: () -> Unit) {
    val c = reynaColors
    Box(
        Modifier
            .fillMaxWidth()
            .clip(CircleShape)
            .border(1.5.dp, c.onSurface, CircleShape)
            .clickable { onClick() }
            .padding(vertical = 14.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)
    }
}
