package com.raisesteady.guard

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import com.raisesteady.guard.ui.components.GuardCard
import com.raisesteady.guard.ui.theme.GuardDimens
import com.raisesteady.guard.ui.theme.GuardTheme
import com.raisesteady.guard.ui.theme.LocalGuardColors

/** Single activity. The setup, status and parent screens arrive with W6; this is the shell. */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            GuardTheme {
                val c = LocalGuardColors.current
                Surface(modifier = Modifier.fillMaxSize(), color = c.bg) {
                    Column(
                        modifier = Modifier
                            .safeDrawingPadding()
                            .verticalScroll(rememberScrollState())
                            .padding(GuardDimens.gap),
                        verticalArrangement = Arrangement.spacedBy(GuardDimens.gap),
                    ) {
                        Text(
                            stringResource(R.string.app_name),
                            style = MaterialTheme.typography.headlineMedium,
                            color = c.text,
                            modifier = Modifier.semantics { heading() },
                        )
                        GuardCard {
                            Text(stringResource(R.string.welcome_title), style = MaterialTheme.typography.titleLarge, color = c.text)
                            Text(stringResource(R.string.welcome_free), style = MaterialTheme.typography.bodyLarge, color = c.text)
                        }
                        Text(stringResource(R.string.honest), style = MaterialTheme.typography.bodyMedium, color = c.text2)
                        Text(stringResource(R.string.about), style = MaterialTheme.typography.bodySmall, color = c.text2)
                    }
                }
            }
        }
    }
}
