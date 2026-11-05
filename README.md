# RaiseSteady Guard

Free, open-source child-safe browsing for Windows and Linux computers: one installer (or package) and one browser extension. A parent installs it once. It makes the child's browsers safer and keeps browsing within an age-based daily time. There is also an app for Android phones and tablets: see [Android](#android).

Made with AI, provided without any guarantee, and supported on a voluntary basis: see [About this project](#about-this-project).

Status: early (version 0.3.2). Tested in automated runs on Windows, Ubuntu and Fedora; not yet tested by families on home computers. In Firefox the extension installs by itself (signed by Mozilla). It is not in the Chrome, Edge and Brave stores yet, so in those browsers only part of it works for now (see [Before it fully works on a home PC](#before-it-fully-works-on-a-home-pc)).

## What it does

**On the child's Windows account** (on Linux: every account on the computer, see [Install on Linux](#install-on-linux)), checked again every 5 minutes:
- Google safe search and YouTube Restricted Mode in Chrome, Edge and Brave; Bing safe search in Edge. In Firefox, and for Bing in Chrome and Brave, safe search comes from the family DNS filter (and from the extension once it is installed).
- the family DNS filter (AdGuard DNS Family) in all four browsers, unless the parent switches it off
- no private or guest windows and no other extensions; pop-ups and notification requests are blocked; developer tools are off under 13
- in Chrome, Edge and Brave the child also cannot delete browsing history or add browser profiles, and dangerous downloads are blocked
- learning and play bookmarks (Khan Academy, TypingClub, TED-Ed and others)
- other browsers (Opera, Tor Browser, Yandex, Vivaldi and more) are closed in the child's account
- PC games and chat apps: Roblox, Minecraft, the Epic Games Launcher with Fortnite, Steam, Battle.net, Riot Client (League of Legends, Valorant), EA app, Ubisoft Connect, Discord, Telegram Desktop and WhatsApp. The parent sets each to Allowed, Game time or Blocked. Game time is a daily limit (at first the video time of the age; extra minutes a parent gives count for games too); apps on game time are closed when it is used up, at bedtime, in homework time and while the internet is paused, and blocked apps are always closed, with a short message to the child and one warning 5 minutes before game time ends. Chat apps start blocked under the minimum age in their own terms (Discord and WhatsApp 13, Telegram 18) and allowed from it

**The browser extension:**
- blocks adult, gambling and filter-bypass sites from open block lists (about 170,000 sites), and checks each page as it opens
- daily browsing time and video time for the child's age, a bedtime, and a break reminder every 20 to 45 minutes
- weekend times (optional): other minutes on Saturday and Sunday, and another bedtime on Friday and Saturday nights
- homework time (optional): a window on school days (Monday to Friday) when only learning sites and the parent's allowed sites open
- "Pause the internet" for 30 minutes, 1 or 2 hours, or until 06:00 the next morning: every site shows a calm "paused" page
- "Ask a parent" on the block page and the time-is-up page: the child can send a request (this site, or more time) with a short note; the parent answers on the parent page, or with the PIN on the spot
- the words the child searches for on Google, Bing, DuckDuckGo, Yahoo, Ecosia, Yandex and YouTube, for the parent's report (on unless the parent switches it off; the extension's window tells the child)
- YouTube with one search box (ours; YouTube's own search bar is hidden there and "/" goes to ours) and videos from chosen learning channels instead of the feed, and no Shorts; for ages 3 to 5, search also shows only the chosen channels. When YouTube's channel feeds do not answer, a list of recent videos built into the extension is shown instead, with the line "Showing saved videos"
- a topic filter, "Sexuality and gender identity topics", on for every age (3 to 17); the parent can switch it off. It blocks searches, pages and YouTube videos about sexual orientation, gender identity and transition, in English, Georgian and Russian
- the topic filter reads the search words, the page title and description, the main headings, the web address and YouTube titles. It uses fixed word and site lists on the PC: no AI, nothing is sent anywhere. It also sees through common spelling tricks (gaps, dots or emoji between letters, digits for letters, look-alike letters, repeated letters, hidden characters, hashtags)
- helplines (findahelpline.com, child helplines on 116 111, Samaritans and others) are never blocked by the extension, at bedtime and in a pause too. The topic page links to findahelpline.com. A parent can allow a site with the PIN
- social networks (TikTok, Instagram, Facebook, X, Snapchat, Reddit, Discord and others) are blocked under 13, the minimum age in their own terms; from 13 they open without the feed
- an ad blocker
- learning sites do not count towards the daily time

**For the parent:** a settings page on the PC, behind a PIN, with the child's open requests first, "Pause the internet", and a 7-day report (time per day, game minutes, and for a picked day the searches and the apps that were closed). Nothing is sent to us.

Default daily times (a parent can change them):

| Age | Browsing | Video | Bedtime |
|---|---|---|---|
| 3-5 | 60 min | 30 min | 19:30-07:00 |
| 6-8 | 90 min | 45 min | 20:00-07:00 |
| 9-12 | 120 min | 60 min | 21:00-07:00 |
| 13-15 | 120 min | 90 min | 22:00-06:30 |
| 16-17 | 120 min | 90 min | 23:00-06:30 |

Video time is part of browsing time, not added to it. The defaults are based on the WHO guidelines for children under 5 (2019) and the Canadian 24-Hour Movement Guidelines for ages 5 to 17.

## What it does not do

- It covers web browsers and the PC games and chat apps listed above. Other programs are not covered.
- On Windows a listed app is found by its file name and by the product name inside the file, so renaming it does not get round it; a changed copy is not covered. On Linux it is found by its program name, Flatpak id or snap name, so a renamed copy in a home folder is not caught. Of the listed apps only Steam, Minecraft, Discord and Telegram Desktop have Linux versions from their makers.
- Closing Steam does not always close a game started from it, and games bought on Steam are not listed one by one. Game time counts while a listed game or launcher is open, also minimized.
- Pause, homework time, searches and "Ask a parent" are in the browser extension, so they work only where it is installed (see below). PC apps are closed by the service in any case.
- Requests stay on the PC: a parent sees them on the parent page of the same computer, not on a phone.
- With "Everyone on this PC" on Windows, and always on Linux, the PC apps, the pause and the searches apply to every account, the parent's too.
- Roblox is covered from roblox.com and from the Microsoft Store (by its install folder); Minecraft only through Mojang's own launcher, not through other launchers.
- The child needs a standard account, not an administrator one (on Linux: no sudo). An administrator can remove it.
- It does not follow clock changes. The day and bedtime use the clock offset set at install, so changing the time zone gives a child no extra time. When summer or winter time starts, change the offset on the parent page.
- The topic filter works on words only. Pictures, videos without a telling title, sound, and pages with few words are not checked. Fixed rules cannot understand meaning: new slang, invented spellings, other languages and indirect wording get through, and a page about something else can be blocked by mistake (a parent can allow it with the PIN). It does not read the body text of pages, so a long article that mentions the topic once is not blocked. Apps other than browsers are not covered. A family DNS filter or another filter set up on the PC or the router is outside the extension and could still block a helpline.
- No filter is perfect. Stay close and talk with your child.

## About this project

- **Made with AI.** The code was written by an AI assistant and checked with automated tests and reviews. No independent expert has audited it.
- **No guarantees.** It is provided "as is", without any guarantee or warranty of any kind, to the extent permitted by law. It may miss unsuitable sites, a child may find a way round it, and it may not work on every PC. Nexodyne Limited and the contributors take no responsibility for any loss or damage from installing or using it, to the extent permitted by law (see also sections 15 and 16 of the [GPL-3.0](LICENSE)).
- **Support is voluntary.** Questions and bug reports are welcome in [Issues](../../issues), but there is no promise of an answer, a fix or future updates.

## Before it fully works on a home PC

On a home PC, Chrome, Edge and Brave install a required extension only from their own stores, and Firefox only one signed by Mozilla.

- **Firefox:** the extension is signed by Mozilla and comes with the installer and the Linux packages (from 0.2.2). Firefox installs it by itself the next time it starts.
- **Chrome, Edge and Brave:** until the extension is published in their stores, the installer sets the browser settings above, but the extension (time limits, page check, YouTube without the feed, ad blocker) does not install.

## Install on Windows

1. Download `RaiseSteadyGuardSetup.exe` from [Releases](../../releases).
2. Release files are not code-signed yet, so Windows may show "Windows protected your PC". Click **More info**, then **Run anyway**.
3. Windows asks for an administrator. Choose the child's Windows account, the child's age and a PIN, and whether to use the family DNS filter.

The parent page is in the Start menu as "Parent settings". Languages: English, Georgian, Russian.

Keep the PIN safe: changing the settings, updating and removing all need it, and there is no way to reset it yet. Five wrong PINs lock it for 15 minutes.

## Install on Linux

For 64-bit Linux with systemd (Intel and AMD, or ARM: take the files marked arm64 or aarch64). From [Releases](../../releases):

- **Ubuntu, Debian, Linux Mint:** `raisesteady-guard_<version>-1_amd64.deb`, then `sudo apt install ./raisesteady-guard_<version>-1_amd64.deb`
- **Fedora and other RPM systems:** `raisesteady-guard-<version>-1.x86_64.rpm`, then `sudo dnf install ./raisesteady-guard-<version>-1.x86_64.rpm`
- **Other Linux (Arch and others):** `raisesteady-guard-<version>-linux-amd64.tar.gz`, unpack it, then `sudo ./install.sh` in that folder

Then run `sudo raisesteady-guard setup`: it shows the terms and asks for the child's age, a PIN and the family DNS filter. The parent page opens from the app menu ("RaiseSteady Guard") or at http://127.0.0.1:47615/parent.

On Linux, browsers read these settings for the whole computer, so they apply to every account on it, including the parent's. It works best on a computer the child uses. Protected browsers: Chrome, Chromium, Edge, Brave and Firefox from their deb, rpm or snap packages, and the Firefox Flatpak installed for all users. Other browsers, and copies of these elsewhere, are closed. A browser that cannot see the settings (a trick some advanced users try) is closed too. Setup and the parent page are in English, Georgian or Russian, from the system language.

## Android

RaiseSteady Guard for Android (version 1.0.0, Android 10 or later) is an app the parent installs on the child's phone or tablet. No account, no cloud and no parent phone: the parent uses a PIN on the child's device. Nothing is sent to us. Status: early; tested in automated runs on Android emulators (Android 10 and 16), not yet by families on real phones.

**What it does:**
- A site filter on the whole device, run as a local VPN that sees only the names of the sites looked up (DNS): adult, gambling and filter-bypass sites are blocked; Google, Bing and DuckDuckGo use safe search; YouTube uses Restricted Mode (Google documents that this also covers the YouTube app; we test the addresses, not every video); social network sites are blocked under 13 (VK and OK under 14); sites mainly about sexuality and gender identity are blocked at every age (the "Sexuality and gender identity topics" switch on the parent page, as in the browser extension). The family DNS filter (AdGuard DNS Family) is on unless the parent switches it off. Helplines are never blocked. The parent can block or allow single sites.
- Apps: each app is Allowed, on Game time or Blocked. Games start on Game time, social and chat apps start blocked under the age in their own terms, known filter-bypass apps (Tor, Psiphon, Opera with its built-in VPN) are blocked. Game time is one daily time for all of them, with a bedtime, weekend times and homework time, a warning 5 minutes before it ends, and a calm screen for the child that says why an app is closed. The parent can pause game-time apps or give 15 or 30 more minutes with the PIN.
- Without the PIN, the app cannot be removed (it is a device admin app) and its own pages in Settings are covered by a PIN screen: App info, its accessibility page, VPN, Private DNS, date and time, developer options and others. The Accessibility list itself stays open, so a child can always turn on TalkBack and other aids.
- A report on the device for the parent: minutes of game time and the apps used most, today and for the last 7 days. The sites opened are not kept.

**What it cannot do on Android** (without becoming the device's owner app, planned for later):
- stop a factory reset or safe mode, both of which remove or switch off the app;
- switch on Always-on VPN or set Private DNS by itself: setup shows the parent where to switch them on;
- protect guest users, other users or a private space (the filter covers one user);
- protect cloned or dual apps and a work profile (they run as another user);
- stop someone who uses USB debugging (adb) from switching it off;
- block installing apps (new games start on Game time; use the Play Store's parental controls so new apps need your OK);
- filter apps that use their own encrypted DNS or fixed addresses, or see inside apps and pages;
- keep time limits on in Advanced Protection mode (Android 16), which switches off accessibility services that are not accessibility tools;
- close a video that keeps playing in picture-in-picture after its app left the screen (planned for 1.1).

Another VPN app replaces the filter unless Always-on VPN is on for RaiseSteady Guard; known VPN and proxy apps start Blocked.

No filter is perfect: stay close and talk with your child about what they see online. Made with AI, provided "as is" without any guarantee, support voluntary: see [About this project](#about-this-project).

**Install from GitHub:**
1. On the child's phone or tablet, open [Releases](../../releases) and download `raisesteady-guard-android-<version>.apk`. The release notes give its certificate SHA-256, to compare if you wish.
2. Open the file. Android asks to allow **Install unknown apps** for the browser or files app you used: allow it, go back and tap **Install**. (You can switch that permission off again afterwards.)
3. Open RaiseSteady Guard and follow the setup (about 5 minutes): the child's age, a PIN, the family DNS filter, then each Android permission with one plain sentence on why it is needed.
4. **Restricted setting:** Android 13 and later protect accessibility access for apps installed from a file. If the switch for RaiseSteady Guard in Accessibility is greyed out or says "Restricted setting", open Settings, Apps, RaiseSteady Guard (App info), tap the menu at the top right, then **Allow restricted settings**, and switch it on again. The setup step has an "Open App info" button for this.
5. For the strongest filter, switch on **Always-on VPN** and **Block connections without VPN** for RaiseSteady Guard in Settings, Network, VPN (the setup step opens it).

Remove: open the app, For the parent (PIN), Turn protection off; then Settings, Apps, RaiseSteady Guard, Uninstall.

## Remove

Windows: Settings, Apps, "RaiseSteady Guard", Uninstall. An administrator and the PIN are needed. Everything it set is removed, and settings that were there before are put back.

Linux: `sudo apt remove raisesteady-guard` or `sudo dnf remove raisesteady-guard` (from the .tar.gz: `sudo /opt/raisesteady-guard/uninstall.sh`). An administrator is needed. Everything it set is removed; Firefox policies set by another tool are put back.

## Privacy

On Android, the same holds: the site filter runs on the device and looks only at site names; with the family DNS filter on, the names go to AdGuard DNS over an encrypted connection; minutes per app are kept on the device for 30 days, without site names.

This program does not send information to other networked systems unless the person installing or operating it chooses it. The two cases below are shown at install, and both can be switched off on the parent page:
- **Family DNS filter** (on unless the parent switches it off): the child's browsers look up site names through [AdGuard DNS](https://adguard-dns.io/en/privacy.html), a free public service. AdGuard sees the site names and your internet address, as any DNS service does. If AdGuard DNS cannot be reached (an outage, or a network that blocks it), sites do not open until it is back or a parent switches the filter off.
- **Chosen YouTube videos** (with Focus mode on): the new tab page and the YouTube start page show videos from the chosen learning channels. To list them, the child's browser fetches each channel's public feed from youtube.com (kept for 6 hours) and the thumbnails from YouTube's image server. YouTube sees your internet address, as when the child opens youtube.com. No account or key is used.

Everything else stays on the PC: minutes used per day, the names of sites visited (not full addresses), blocked pages, search words, game minutes and the apps that were closed, kept for 30 days, and the child's requests, kept for 7 days, all readable by administrators only. Search words are read from the address of a search result page only (never from the page), cut to 100 characters, at most 50 different ones a day; a parent can switch this off on the parent page, and then nothing is recorded. The child is told in the extension's window. The block lists are built into the extension, and pages are checked on the PC, never sent anywhere.

## Code signing policy

We plan to sign the Windows release files with SignPath Foundation's free certificate for open-source projects. Signing is not active yet: until it is, release files are unsigned.

Once active: Free code signing provided by [SignPath.io](https://about.signpath.io/), certificate by [SignPath Foundation](https://signpath.org/).

- Committers and reviewers: [tengotengo23](https://github.com/tengotengo23) (project owner).
- Approvers: [tengotengo23](https://github.com/tengotengo23). Every signing request will be approved by hand.
- Only files built from this repository by its GitHub build will be signed.

## Licence

The code is under the [GNU General Public License v3.0](LICENSE). The block-list data keeps its own licences: see [SOURCES.md](SOURCES.md) and [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt).

The names "RaiseSteady" and "RaiseSteady Guard" are not covered by the licence: a changed version needs its own name.

## Build and change it

See [docs/DEVELOPING.md](docs/DEVELOPING.md).
