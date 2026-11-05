package com.raisesteady.guard.lists

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File
import java.io.InputStream
import java.net.InetAddress

/** The data the filter reads from the app's assets (copied in at build time by SharedDataTask). */
class GuardData(
    val lists: DomainLists,
    val social: SocialAges,
    val helplines: List<String>,
    val dohIps: List<InetAddress>,
) {
    companion object {
        /** [open] opens an asset by name (AssetManager::open in the app, files in JVM tests). */
        fun load(open: (String) -> InputStream): GuardData {
            val lists = open("lists.bin").use { DomainLists.read(it) }
            val social = SocialAges.parse(open("social.json").use { it.reader().readText() })
            val topics = Json.parseToJsonElement(open("topics.json").use { it.reader().readText() }).jsonObject
            val helplines = topics["helplines"]!!.jsonArray.map { it.jsonPrimitive.content.lowercase() }
            val ips = Json.parseToJsonElement(open("doh-ips.json").use { it.reader().readText() }).jsonArray
                .map { InetAddress.getByName(it.jsonPrimitive.content) } // numeric literals only: no lookup
            return GuardData(lists, social, helplines, ips)
        }

        fun load(dir: File): GuardData = load { dir.resolve(it).inputStream() }
    }
}
