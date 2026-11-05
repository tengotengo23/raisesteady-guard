// Build-time helpers: copies kidguard/shared and ext/rules data into the app's assets.
// Needs only the JDK (no Node): JSON is read with Gradle's bundled groovy.json.
plugins {
    `kotlin-dsl`
}

dependencies {
    compileOnly("com.android.tools.build:gradle-api:${libs.versions.agp.get()}")
    implementation(localGroovy())
}

gradlePlugin {
    plugins {
        create("sharedData") {
            id = "com.raisesteady.guard.shared-data"
            implementationClass = "com.raisesteady.guard.build.SharedDataPlugin"
        }
    }
}
