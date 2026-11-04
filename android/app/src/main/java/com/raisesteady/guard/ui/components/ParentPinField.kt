package com.raisesteady.guard.ui.components

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import com.raisesteady.guard.ui.theme.GuardDimens
import com.raisesteady.guard.ui.theme.LocalGuardColors

/** The PIN field (digits, hidden; TalkBack reads its label). Placeholder look: desk C finishes it (W6). */
@Composable
fun ParentPinField(value: String, onChange: (String) -> Unit, label: String, error: String?) {
    OutlinedTextField(
        value = value,
        onValueChange = { v -> onChange(v.filter(Char::isDigit).take(8)) },
        label = { Text(label) },
        singleLine = true,
        isError = error != null,
        supportingText = error?.let { { Text(it) } },
        visualTransformation = PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
        modifier = Modifier.fillMaxWidth(),
    )
}

/** The main pill button (min height 48 dp). */
@Composable
fun MainButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val c = LocalGuardColors.current
    Button(
        onClick = onClick,
        shape = RoundedCornerShape(50),
        colors = ButtonDefaults.buttonColors(containerColor = c.accent, contentColor = c.accentText),
        modifier = modifier.fillMaxWidth().heightIn(min = GuardDimens.minTouch),
    ) { Text(text) }
}

/** A second pill button with an outline. */
@Composable
fun SecondButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val c = LocalGuardColors.current
    OutlinedButton(
        onClick = onClick,
        shape = RoundedCornerShape(50),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = c.text),
        modifier = modifier.fillMaxWidth().heightIn(min = GuardDimens.minTouch),
    ) { Text(text) }
}
