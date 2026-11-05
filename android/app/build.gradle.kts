// RaiseSteady Guard for Android: the app (com.raisesteady.guard). GPL-3.0-only.
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    id("com.raisesteady.guard.shared-data")
}

// version.properties: versionName; versionCode = major*1_000_000 + minor*1_000 + patch.
val versionProps = Properties().apply { rootProject.file("version.properties").inputStream().use { load(it) } }
val guardVersion: String = versionProps.getProperty("versionName")
val guardVersionCode: Int = guardVersion.split(".").map { it.toInt() }.let { (major, minor, patch) ->
    major * 1_000_000 + minor * 1_000 + patch
}

// Release signing only when every env var is set (key outside the repo; tools/sign-release.sh sets them).
val keystorePath: String? = providers.environmentVariable("GUARD_KEYSTORE").orNull
val keystorePassword: String? = providers.environmentVariable("GUARD_KEYSTORE_PASSWORD").orNull
val keyAliasName: String? = providers.environmentVariable("GUARD_KEY_ALIAS").orNull
val keyPasswordValue: String? = providers.environmentVariable("GUARD_KEY_PASSWORD").orNull
val canSign = listOf(keystorePath, keystorePassword, keyAliasName, keyPasswordValue).all { !it.isNullOrEmpty() }

android {
    namespace = "com.raisesteady.guard"
    // Compose 1.12 and core 1.19 need compileSdk 37; the app still targets 36 (Play rule), minSdk 29.
    compileSdk = 37

    defaultConfig {
        applicationId = "com.raisesteady.guard"
        minSdk = 29
        targetSdk = 36
        versionCode = guardVersionCode
        versionName = guardVersion
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        if (canSign) {
            create("release") {
                storeFile = file(keystorePath!!)
                storePassword = keystorePassword
                keyAlias = keyAliasName
                keyPassword = keyPasswordValue
                storeType = "pkcs12"
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            vcsInfo.include = false
            if (canSign) signingConfig = signingConfigs.getByName("release")
        }
    }

    // Reproducible builds: no dependency block, no VCS info, no build time.
    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    lint {
        abortOnError = true
        checkDependencies = false
        error += setOf("MissingTranslation", "HardcodedText", "ContentDescription")
        // Versions are pinned on purpose (exact, at least two weeks old); updates are a decision, not a lint finding.
        disable += setOf("GradleDependency", "NewerVersionAvailable", "AndroidGradlePluginVersion")
        // targetSdk 36 is the level Google Play asks for (since 31 Aug 2026); 37 after emulator tests on API 37.
        disable += "OldTargetApi"
    }

    packaging {
        resources {
            // JNA's desktop native libraries: pcap4j uses only JNA's Platform class on Android.
            excludes += setOf("com/sun/jna/**/libjnidispatch.*", "com/sun/jna/**/*.a")
            // dnsjava's resolver plug-in for desktop Java 18+ (java.net.spi); not used on Android.
            excludes += "META-INF/services/java.net.spi.InetAddressResolverProvider"
        }
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }
}

androidComponents {
    onVariants(selector().withBuildType("release")) { variant ->
        variant.outputs.forEach { output ->
            (output as? com.android.build.api.variant.impl.VariantOutputImpl)
                ?.outputFileName?.set("raisesteady-guard-android-$guardVersion.apk")
        }
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.foundation)
    implementation(libs.compose.material3)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.androidx.datastore)
    // DNS filter: IP/UDP packets (pcap4j, as AdAway), DNS messages (dnsjava), DoH (OkHttp).
    // pcap4j reads com.sun.jna.Platform when it starts, so JNA's classes stay (its native libraries do not).
    implementation(libs.pcap4j.core)
    implementation(libs.pcap4j.packetfactory.static)
    implementation(libs.jna)
    implementation(libs.slf4j.nop)
    implementation(libs.dnsjava)
    implementation(libs.okhttp)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}
