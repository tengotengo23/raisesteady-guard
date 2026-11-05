package com.raisesteady.guard.build

import com.android.build.api.variant.ApplicationAndroidComponentsExtension
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.api.file.Directory
import org.gradle.api.provider.Provider
import org.gradle.api.tasks.InputDirectory
import org.gradle.api.tasks.PathSensitive
import org.gradle.api.tasks.PathSensitivity
import org.gradle.api.tasks.testing.Test
import org.gradle.process.CommandLineArgumentProvider

/**
 * Adds one [SharedDataTask] per variant and puts its output into the variant's assets.
 * The debug unit tests get the same folder as the system property `guard.assets`.
 */
class SharedDataPlugin : Plugin<Project> {
    override fun apply(project: Project) {
        // kidguard/android is the Gradle root; the shared data lives one folder up, in kidguard/.
        val kidguard = project.rootProject.layout.projectDirectory.dir("..")
        project.pluginManager.withPlugin("com.android.application") {
            val components = project.extensions.getByType(ApplicationAndroidComponentsExtension::class.java)
            components.onVariants { variant ->
                val name = "guardSharedData" + variant.name.replaceFirstChar { it.uppercase() }
                val task = project.tasks.register(name, SharedDataTask::class.java) {
                    kidguardDir.set(kidguard)
                    sources.from(SharedDataTask.INPUTS.map { kidguard.file(it) })
                }
                variant.sources.assets?.addGeneratedSourceDirectory(task, SharedDataTask::outputDir)
                if (variant.name == "debug") {
                    val dir = task.flatMap { it.outputDir }
                    project.tasks.withType(Test::class.java)
                        .matching { it.name == "testDebugUnitTest" }
                        .configureEach { jvmArgumentProviders.add(AssetsDirArg(dir)) }
                }
            }
        }
    }
}

/** Passes the generated asset folder to the JVM unit tests (and makes them depend on it). */
class AssetsDirArg(
    @get:InputDirectory @get:PathSensitive(PathSensitivity.RELATIVE) val dir: Provider<Directory>,
) : CommandLineArgumentProvider {
    override fun asArguments(): Iterable<String> = listOf("-Dguard.assets=" + dir.get().asFile.absolutePath)
}
