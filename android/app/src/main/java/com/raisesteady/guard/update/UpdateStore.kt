package com.raisesteady.guard.update

import androidx.datastore.core.DataStore
import androidx.datastore.core.DataStoreFactory
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import com.raisesteady.guard.data.JsonSerializer
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.stateIn
import kotlinx.serialization.Serializable
import java.io.File

/** What the updater remembers: no family data, only times and a count (updater.json, beside Guard's other files). */
@Serializable
data class UpdateStatus(
    /** When the last successful check finished (ms), 0 if never. */
    val lastCheck: Long = 0,
    /** When the last failed check finished (ms), 0 if never. */
    val lastFailure: Long = 0,
    /** Failed checks in a row, for the back-off. */
    val failures: Int = 0,
    /** When the next check is due (ms). */
    val nextCheckAt: Long = 0,
    /** One line on the last result (for the debug status and logs). */
    val lastNote: String = "",
)

class UpdateStore(dir: File, scope: CoroutineScope) {
    private val store: DataStore<UpdateStatus> = DataStoreFactory.create(
        serializer = JsonSerializer(UpdateStatus.serializer(), UpdateStatus()),
        corruptionHandler = ReplaceFileCorruptionHandler { UpdateStatus() },
        scope = scope,
        produceFile = { File(dir, "updater.json") },
    )

    val status: StateFlow<UpdateStatus> = store.data.stateIn(scope, SharingStarted.Eagerly, UpdateStatus())

    suspend fun read(): UpdateStatus = store.data.first()

    suspend fun update(f: (UpdateStatus) -> UpdateStatus): UpdateStatus = store.updateData(f)
}
