package com.raisesteady.guard.ui.child

import androidx.annotation.DrawableRes
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import com.raisesteady.guard.R
import com.raisesteady.guard.rules.Reason
import com.raisesteady.guard.ui.components.Body
import com.raisesteady.guard.ui.components.GuardCard
import com.raisesteady.guard.ui.components.MainButton
import com.raisesteady.guard.ui.components.Note
import com.raisesteady.guard.ui.components.Page
import com.raisesteady.guard.ui.components.ParentPinField
import com.raisesteady.guard.ui.components.RsEmoji
import com.raisesteady.guard.ui.components.SecondButton
import com.raisesteady.guard.ui.components.Title
import kotlinx.coroutines.launch

/** The RS emoji of each reason (design 4.3). */
@DrawableRes
fun emojiFor(reason: Reason): Int = when (reason) {
    Reason.TIME_UP -> R.drawable.rs_hourglass
    Reason.BEDTIME -> R.drawable.rs_moon
    Reason.HOMEWORK -> R.drawable.rs_books
    Reason.PAUSED -> R.drawable.rs_alarm
    Reason.BLOCKED -> R.drawable.rs_lock
}

/**
 * The closed-app screen (design 4.3), calm, for the child: why the app is closed and how to ask a
 * parent. "For the parent" takes the PIN, then "15 more minutes" / "30 more minutes" / "Cancel"
 * (not for Blocked apps).
 */
@Composable
fun BlockScreen(
    title: String,
    reason: Reason,
    reasonText: String,
    onHome: () -> Unit,
    onMoreTime: suspend (pin: String, minutes: Int) -> String?,
) {
    val scope = rememberCoroutineScope()
    val canGiveTime = reason != Reason.BLOCKED
    var parent by remember { mutableStateOf(false) }
    var pin by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    Page(key = "block-$reason-$parent", bottom = {
        if (!parent) {
            MainButton(stringResource(R.string.block_home), onHome)
            if (canGiveTime) SecondButton(stringResource(R.string.for_parent), { parent = true })
        } else {
            for (m in listOf(15, 30)) {
                MainButton(stringResource(if (m == 15) R.string.more_15 else R.string.more_30), {
                    scope.launch {
                        error = onMoreTime(pin, m)
                        if (error != null) pin = ""
                    }
                })
            }
            SecondButton(stringResource(R.string.cancel), { parent = false; pin = ""; error = null })
        }
    }) {
        RsEmoji(emojiFor(reason), modifier = Modifier.testTag("emoji"))
        Title(title)
        GuardCard {
            Body(reasonText)
            if (canGiveTime) Note(stringResource(R.string.block_ask))
        }
        if (parent) ParentPinField(pin, { pin = it; error = null }, stringResource(R.string.pin_label), error)
    }
}

/** The settings cover (design 4.3): a page that changes the protection needs the parent PIN. */
@Composable
fun GateScreen(onUnlock: suspend (String) -> String?, onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    var pin by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    Page(key = "gate", bottom = {
        MainButton(stringResource(R.string.gate_unlock), {
            scope.launch {
                error = onUnlock(pin)
                if (error != null) pin = ""
            }
        })
        SecondButton(stringResource(R.string.gate_back), onBack)
    }) {
        RsEmoji(R.drawable.rs_lock)
        Title(stringResource(R.string.gate_title))
        Body(stringResource(R.string.gate_text))
        ParentPinField(pin, { pin = it; error = null }, stringResource(R.string.pin_label), error)
    }
}
