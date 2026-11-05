package com.raisesteady.guard.guard

import android.content.Context
import android.view.accessibility.AccessibilityNodeInfo

/**
 * Which windows the PIN screen covers (design 3.6). Only these packages are ever read: Settings,
 * the permission controller, the package installer and the VPN dialogs. In Settings only our own
 * pages are covered (review S2): App info for this app (titled with our label or "App info", or
 * showing our label with its Force stop button), our App battery page, VPN settings and its gear
 * page, our own accessibility page and its turn-off dialog, and the guarded pages found by their
 * title (Private DNS, Date and time, Developer options, Multiple users, Private space, Reset
 * options, Device admin apps). A list that merely shows our name (Apps, Notifications, Battery
 * usage, search results) stays open. The short dialogs of the package installer, the permission
 * controller and the VPN dialogs are covered when they show our label; any VPN consent dialog is
 * covered (another app's VPN would replace ours). The parent's PIN opens them for 10 minutes.
 *
 * The Accessibility list itself is never covered, though it shows our row: a child must be free
 * to turn on TalkBack and other aids (director's decision D1). Only our own page there (its
 * "Use RaiseSteady Guard" switch, or the page titled with our label) and its turn-off dialog are.
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
        // Android 14+ hides the rows of Device admin apps from accessibility services that are not
        // accessibility tools (accessibilityDataSensitive), so that page is found by its title (run 41925770506).
        "manage_device_admin", "device_admin_settings_title",
    )

    /** The Accessibility list: its title and the heading of its downloaded services (AOSP names). */
    val A11Y_LIST_RESOURCES = listOf(
        "accessibility_settings", "accessibility_settings_title",
        "user_installed_services_category_title", "accessibility_services_title",
    )

    /** Texts only on our own accessibility page and its turn-off dialog: "%1$s" is our label (AOSP names). */
    val OUR_A11Y_RESOURCES = listOf(
        "accessibility_service_primary_switch_title", "accessibility_service_master_switch_title",
        "disable_service_title", "accessibility_shortcut_title", "accessibility_service_shortcut_title",
    )

    val FALLBACK_A11Y_LIST = setOf(
        "Accessibility", "Downloaded apps", "Downloaded services", "Installed apps", "Installed services",
        "Специальные возможности", "Спец. возможности", "Скачанные приложения", "Установленные приложения", "Скачанные сервисы",
    )

    val FALLBACK_OUR_A11Y = setOf(
        "Use $LABEL", "Stop $LABEL?", "Turn off $LABEL?", "$LABEL shortcut",
        "Использовать $LABEL", "Остановить $LABEL?", "Отключить $LABEL?",
    )

    /** What the guard compares window titles and texts with, in the device's language plus fallbacks. */
    data class Marks(
        val pages: Set<String>,
        val a11yList: Set<String>,
        val ourA11y: Set<String>,
        val ownPages: Set<String> = FALLBACK_OWN_PAGES,
        val appInfoButtons: Set<String> = FALLBACK_APP_INFO_BUTTONS,
    ) {
        internal val pagesN = pages.map(::normalize).toSet()
        internal val a11yListN = a11yList.map(::normalize).toSet()
        internal val ourA11yN = ourA11y.map(::normalize).toSet()
        internal val ownPagesN = ownPages.map(::normalize).toSet()
        internal val appInfoButtonsN = appInfoButtons.map(::normalize).toSet()

        companion object {
            val FALLBACK = Marks(FALLBACK_TITLES, FALLBACK_A11Y_LIST, FALLBACK_OUR_A11Y)
        }
    }

    /** Fallback titles when the Settings app's resources cannot be read (en, ru). */
    val FALLBACK_TITLES = setOf(
        "Private DNS", "Date & time", "Date and time", "Developer options", "Multiple users", "Private space", "Reset options",
        "Device admin apps", "Device admin app", "Device admin",
        "Персональный DNS-сервер", "Дата и время", "Для разработчиков", "Несколько пользователей", "Сброс настроек",
        // Georgian (review N1), for when the Settings app's own strings cannot be read.
        "პირადი DNS", "თარიღი და დრო", "დეველოპერის პარამეტრები", "რამდენიმე მომხმარებელი", "გადატვირთვის პარამეტრები",
    )

    /** Titles of our own pages in Settings, when our label is on them (AOSP names). */
    val OWN_PAGE_RESOURCES = listOf(
        "application_info_label", "app_info_title", "vpn_settings_title", "vpn_title", "app_battery_usage_title",
    )

    /** The Force stop button: with our label, it is App info for this app whatever its title (AOSP names). */
    val APP_INFO_BUTTON_RESOURCES = listOf("force_stop")

    val FALLBACK_OWN_PAGES = setOf(
        "App info", "App information", "VPN", "App battery usage",
        "О приложении", "Сведения о приложении", "Расход заряда приложением",
        "აპის ინფორმაცია", "აპის შესახებ",
    )

    val FALLBACK_APP_INFO_BUTTONS = setOf("Force stop", "Остановить", "Принудительно остановить", "ძალით შეჩერება", "იძულებით შეჩერება")

    fun normalize(t: String) = t.trim().lowercase().replace("&", "and").replace(Regex("\\s+"), " ")

    /**
     * Pure: whether to cover a window of [pkg] with the [titles] known for it (the event's and the
     * window's) and its on-screen [texts].
     */
    fun shouldCover(pkg: String, titles: List<String?>, texts: List<String>, marks: Marks, unlocked: Boolean): Boolean {
        if (pkg !in PACKAGES || unlocked) return false
        if (pkg == VPN_DIALOGS) return true
        val known = titles.filterNotNull().filter { it.isNotBlank() }
        val shown = texts.any { it.contains(LABEL) } || known.any { it.contains(LABEL) }
        if (known.any { normalize(it) in marks.pagesN }) return true
        if (!shown) return false
        if (pkg != SETTINGS) return true // the installer's and the permission controller's short dialogs
        if (isOurA11yPage(known, texts, marks)) return true
        // The Accessibility list shows our row next to TalkBack and the other aids: never covered (D1).
        if (isA11yList(known, texts, marks)) return false
        // Only our own pages (S2): a list that just shows our name (Apps, Notifications, search) stays open.
        return known.any { normalize(it) == normalize(LABEL) || normalize(it) in marks.ownPagesN } ||
            texts.any { normalize(it) in marks.appInfoButtonsN }
    }

    /** One title (the event's): kept for the tests and callers with a single title. */
    fun shouldCover(pkg: String, title: String?, texts: List<String>, marks: Marks, unlocked: Boolean): Boolean =
        shouldCover(pkg, listOf(title), texts, marks, unlocked)

    /** Our own accessibility page (titled with our label, or its "Use ..." switch) or its turn-off dialog. */
    fun isOurA11yPage(titles: List<String>, texts: List<String>, marks: Marks): Boolean =
        titles.any { normalize(it) == normalize(LABEL) } || texts.any { normalize(it) in marks.ourA11yN }

    /** The Accessibility list (or its Downloaded apps part): by its title, or its heading on screen. */
    fun isA11yList(titles: List<String>, texts: List<String>, marks: Marks): Boolean =
        titles.any { normalize(it) in marks.a11yListN } || texts.any { normalize(it) in marks.a11yListN }

    /** The guarded titles and the accessibility marks in the device's language, read from the Settings app, plus the fallbacks. */
    fun marks(context: Context): Marks {
        val pages = HashSet(FALLBACK_TITLES)
        val list = HashSet(FALLBACK_A11Y_LIST)
        val ours = HashSet(FALLBACK_OUR_A11Y)
        val own = HashSet(FALLBACK_OWN_PAGES)
        val buttons = HashSet(FALLBACK_APP_INFO_BUTTONS)
        runCatching {
            val res = context.packageManager.getResourcesForApplication(SETTINGS)
            fun read(name: String, vararg args: Any): String? {
                @Suppress("DiscouragedApi") // the only way to read another app's strings by name
                val id = res.getIdentifier(name, "string", SETTINGS)
                if (id == 0) return null
                return runCatching { if (args.isEmpty()) res.getString(id) else res.getString(id, *args) }.getOrNull()?.takeIf { it.isNotBlank() }
            }
            TITLE_RESOURCES.forEach { n -> read(n)?.let(pages::add) }
            A11Y_LIST_RESOURCES.forEach { n -> read(n)?.let(list::add) }
            OUR_A11Y_RESOURCES.forEach { n -> read(n, LABEL)?.takeIf { it.contains(LABEL) }?.let(ours::add) }
            OWN_PAGE_RESOURCES.forEach { n -> read(n)?.let(own::add) }
            APP_INFO_BUTTON_RESOURCES.forEach { n -> read(n)?.let(buttons::add) }
        }
        return Marks(pages, list, ours, own, buttons)
    }

    /** The texts and descriptions in a window (at most [max] nodes, breadth first). */
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
            n.contentDescription?.toString()?.takeIf { it.isNotBlank() }?.let(out::add)
            for (i in 0 until n.childCount) n.getChild(i)?.let(queue::add)
        }
        return out
    }
}
