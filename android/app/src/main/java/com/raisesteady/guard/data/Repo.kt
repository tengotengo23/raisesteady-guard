package com.raisesteady.guard.data

import android.content.Context
import androidx.datastore.core.CorruptionException
import androidx.datastore.core.DataStore
import androidx.datastore.core.DataStoreFactory
import androidx.datastore.core.Serializer
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.stateIn
import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import java.io.File
import java.io.InputStream
import java.io.OutputStream

/** JSON files for DataStore (kotlinx.serialization). Unknown keys are ignored, so older files still load. */
class JsonSerializer<T>(private val serializer: KSerializer<T>, override val defaultValue: T) : Serializer<T> {
    override suspend fun readFrom(input: InputStream): T {
        val text = input.readBytes().decodeToString()
        if (text.isBlank()) return defaultValue
        return try {
            json.decodeFromString(serializer, text)
        } catch (e: SerializationException) {
            throw CorruptionException("unreadable ${serializer.descriptor.serialName}", e)
        } catch (e: IllegalArgumentException) {
            throw CorruptionException("unreadable ${serializer.descriptor.serialName}", e)
        }
    }

    override suspend fun writeTo(t: T, output: OutputStream) {
        output.write(json.encodeToString(serializer, t).encodeToByteArray())
    }

    companion object {
        val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }
    }
}

/**
 * The three files Guard keeps, all on this device (no backup, allowBackup=false): settings.json
 * (what the parent set, the PIN hash), state.json (pause, more time, PIN lock, settings unlock),
 * usage.json (30 days of minutes; no site names). One process.
 */
class Repo(private val dir: File, private val scope: CoroutineScope) {
    constructor(context: Context, scope: CoroutineScope) : this(File(context.filesDir, "datastore"), scope)

    private fun <T> store(name: String, ser: KSerializer<T>, default: T): DataStore<T> = DataStoreFactory.create(
        serializer = JsonSerializer(ser, default),
        corruptionHandler = ReplaceFileCorruptionHandler { default },
        scope = scope,
        produceFile = { File(dir, name) },
    )

    private val settingsStore = store("settings.json", Settings.serializer(), Settings())
    private val stateStore = store("state.json", State.serializer(), State())
    private val usageStore = store("usage.json", Usage.serializer(), Usage())

    val settings: StateFlow<Settings> = settingsStore.data.stateIn(scope, SharingStarted.Eagerly, Settings())
    val state: StateFlow<State> = stateStore.data.stateIn(scope, SharingStarted.Eagerly, State())
    val usage: StateFlow<Usage> = usageStore.data.stateIn(scope, SharingStarted.Eagerly, Usage())

    suspend fun updateSettings(f: (Settings) -> Settings): Settings = settingsStore.updateData(f)
    suspend fun updateState(f: (State) -> State): State = stateStore.updateData(f)
    suspend fun updateUsage(f: (Usage) -> Usage): Usage = usageStore.updateData(f)

    /** Reads the files (not the cached flows): for code that must see what is stored now. */
    suspend fun readSettings(): Settings = settingsStore.data.first()
    suspend fun readState(): State = stateStore.data.first()
}
