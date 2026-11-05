// Plugin versions live in gradle/libs.versions.toml (exact versions, no ranges).
plugins {
    alias(libs.plugins.android.application) apply false
    // AGP 9 has built-in Kotlin; this line only picks the Kotlin Gradle plugin version it uses.
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
}
