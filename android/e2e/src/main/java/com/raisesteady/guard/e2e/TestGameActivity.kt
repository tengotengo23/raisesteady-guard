package com.raisesteady.guard.e2e

import android.app.Activity
import android.os.Bundle
import android.view.Gravity
import android.widget.TextView

/** "Test game": the app Guard's game time limits in the end-to-end tests. */
class TestGameActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(TextView(this).apply { setText(R.string.test_game); gravity = Gravity.CENTER; textSize = 24f })
    }
}
