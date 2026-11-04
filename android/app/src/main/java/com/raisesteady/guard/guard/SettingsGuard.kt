package com.raisesteady.guard.guard

import android.content.Context
import android.view.accessibility.AccessibilityNodeInfo

/**
 * Which windows the PIN screen covers (design 3.6). Only these packages are ever read: Settings,
 * the permission controller, the package installer and the VPN dialogs. A window is covered when
 * its text shows our label (App info, App battery, the uninstall dialog, Device admin apps, our
 * Accessibility page, VPN settings and its gear dialog), when it is a VPN consent dialog (another
 * app's VPN would replace ours), or when its title is one of the guarded pages (Private DNS, Date
 * and time, Developer options, Multiple users, Private space, Reset options). The parent's PIN
 * opens them for 10 minutes.
 */
object SettingsGuard {
    /** Our label: the same in every language. */
    const val LABEL = "RaiseSteady Guard"

    val PACKAGES = setOf(
        "com.android.settings",
        "com.google.android.permissioncontroller",
        "com.android.permissioncontroller",
        "com.google.android.packageinstaller",
        "com.android.packageinstaller",
        VPN_DIALOGS,
    )
    const val VPN_DIALOGS = "com.android.vpndialogs"
    const val SETTINGS = "com.android.settings"

    /** How long the right PIN opens the guarded pages. */
    const val UNLOCK_MS = 10 * 60 * 1000L

    /** Names of the guarded page titles in the Settings app's own resources (AOSP names, several per page across versions). */
    val TITLE_RESOURCES = listOf(
        "select_private_dns_configuration_title", "private_dns_preference_title",
        "date_and_time", "date_and_time_settings_title",
        "development_settings_title",
        "user_settings_title",
        "private_space_title",
        "reset_dashboard_title",
    )

    /** Fallback titles when the Settings app's resources cannot be read (en, ru). */
    val FALLBACK_TITLES = setOf(
        "Private DNS", "Date & time", "Date and time", "Developer options", "Multiple users", "Private space", "Reset options",
        "Персональный DNS-сервер", "Дата и время", "Для разработчиков", "Несколько пользователей", "Сброс настроек",
    )

    fun normalize(t: String) = t.trim().lowercase().replace("&", "and").replace(Regex("\\s+"), " ")

    /** Pure: whether to cover a window of [pkg] with [title] and on-screen [texts]. */
    fun shouldCover(pkg: String, title: String?, texts: List<String>, titles: Set<String>, unlocked: Boolean): Boolean {
        if (pkg !in PACKAGES || unlocked) return false
        if (pkg == VPN_DIALOGS) return true
        if (texts.any { it.contains(LABEL) } || title?.contains(LABEL) == true) return true
        if (title == null) return false
        val t = normalize(title)
        return titles.any { normalize(it) == t }
    }

    /** The guarded titles in the device's language, read from the Settings app, plus the fallbacks. */
    fun titles(context: Context): Set<String> {
        val out = HashSet(FALLBACK_TITLES)
        runCatching {
            val res = context.packageManager.getResourcesForApplication(SETTINGS)
            for (name in TITLE_RESOURCES) {
                @Suppress("DiscouragedApi") // the only way to read another app's strings by name
                val id = res.getIdentifier(name, "string", SETTINGS)
                if (id != 0) runCatching { res.getString(id) }.getOrNull()?.takeIf { it.isNotBlank() }?.let(out::add)
            }
        }
        return out
    }

    /** The texts in a window (at most [max] nodes, breadth first). */
    fun texts(root: AccessibilityNodeInfo?, max: Int = 400): List<String> {
        if (root == null) return emptyList()
        val out = ArrayList<String>()
        val queue = ArrayDeque<AccessibilityNodeInfo>()
        queue.add(root)
        var seen = 0
        while (queue.isNotEmpty() && seen < max) {
            val n = queue.removeFirst()
            seen++
            n.text?.toString()?.takeIf { it.isNotBlank() }?.let(out::add)
            for (i in 0 until n.childCount) n.getChild(i)?.let(queue::add)
        }
        return out
    }
}
