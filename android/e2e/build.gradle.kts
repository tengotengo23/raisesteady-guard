// A tiny test app (com.raisesteady.guard.e2e, a "game") that holds the end-to-end tests.
// It is another UID: its DNS goes through Guard's VPN, Guard limits it as a game, and
// instrumenting it never restarts Guard's own services.
plugins {
    alias(libs.plugins.android.application)
}

android {
    namespace = "com.raisesteady.guard.e2e"
    // Compose 1.12 and core 1.19 need compileSdk 37; the app still targets 36 (Play rule), minSdk 29.
    compileSdk = 37

    defaultConfig {
        applicationId = "com.raisesteady.guard.e2e"
        minSdk = 29
        targetSdk = 36
        versionCode = 1
        versionName = "1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    androidTestImplementation(libs.junit)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.androidx.test.uiautomator)
}
