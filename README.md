# RaiseSteady Guard

Free, open-source child-safe browsing for a Windows PC: one installer and one browser extension. A parent installs it once. It makes the child's browsers safer and keeps browsing within an age-based daily time.

Status: early (version 0.1.0). Tested in automated runs on Windows; not yet tested by families on home PCs. The extension is not in the browser stores yet, so on a home PC only part of it works for now (see [Before it fully works on a home PC](#before-it-fully-works-on-a-home-pc)).

## What it does

**On the child's Windows account**, checked again every 5 minutes:
- Google safe search and YouTube Restricted Mode in Chrome, Edge and Brave; Bing safe search in Edge. In Firefox, and for Bing in Chrome and Brave, safe search comes from the family DNS filter (and from the extension once it is installed).
- the family DNS filter (AdGuard DNS Family) in all four browsers, unless the parent switches it off
- no private or guest windows and no other extensions; pop-ups and notification requests are blocked; developer tools are off under 13
- in Chrome, Edge and Brave the child also cannot delete browsing history or add browser profiles, and dangerous downloads are blocked
- learning and play bookmarks (Khan Academy, TypingClub, TED-Ed and others)
- other browsers (Opera, Tor Browser, Yandex, Vivaldi and more) are closed in the child's account

**The browser extension:**
- blocks adult, gambling and filter-bypass sites from open block lists (about 170,000 sites), and checks each page as it opens
- daily browsing time and video time for the child's age, a bedtime, and a break reminder every 20 to 45 minutes
- YouTube with a search box and videos from chosen learning channels instead of the feed, and no Shorts; for ages 3 to 5, search also shows only the chosen channels
- social networks (TikTok, Instagram, Facebook, X, Snapchat, Reddit, Discord and others) are blocked under 13, the minimum age in their own terms; from 13 they open without the feed
- an ad blocker
- learning sites do not count towards the daily time

**For the parent:** a settings page on the PC, behind a PIN, with a 7-day report. Nothing is sent to us.

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

- It protects web browsers only. Games, chat apps and other programs are not covered.
- The child needs a standard Windows account, not an administrator one. An administrator can remove it.
- It does not follow clock changes. The day and bedtime use the clock offset set at install, so changing the Windows time zone gives a child no extra time. When summer or winter time starts, change the offset on the parent page.
- No filter is perfect. Stay close and talk with your child.

## Before it fully works on a home PC

On a home PC, Chrome, Edge and Brave install a required extension only from their own stores, and Firefox only one signed by Mozilla. Until the extension is published there, the installer sets the browser settings above, but the extension (time limits, page check, YouTube without the feed, ad blocker) does not install.

## Install

1. Download `RaiseSteadyGuardSetup.exe` from [Releases](../../releases).
2. Release files are not code-signed yet, so Windows may show "Windows protected your PC". Click **More info**, then **Run anyway**.
3. Windows asks for an administrator. Choose the child's Windows account, the child's age and a PIN, and whether to use the family DNS filter.

The parent page is in the Start menu as "Parent settings". Languages: English, Georgian, Russian.

Keep the PIN safe: changing the settings, updating and removing all need it, and there is no way to reset it yet. Five wrong PINs lock it for 15 minutes.

## Remove

Settings, Apps, "RaiseSteady Guard", Uninstall. An administrator and the PIN are needed. Everything it set is removed, and settings that were there before are put back.

## Privacy

This program does not send information to other networked systems unless the person installing or operating it chooses it. The two cases below are shown at install, and both can be switched off on the parent page:
- **Family DNS filter** (on unless the parent switches it off): the child's browsers look up site names through [AdGuard DNS](https://adguard-dns.io/en/privacy.html), a free public service. AdGuard sees the site names and your internet address, as any DNS service does. If AdGuard DNS cannot be reached (an outage, or a network that blocks it), sites do not open until it is back or a parent switches the filter off.
- **Chosen YouTube videos** (with Focus mode on): the new tab page and the YouTube start page show videos from the chosen learning channels. To list them, the child's browser fetches each channel's public feed from youtube.com (kept for 6 hours) and the thumbnails from YouTube's image server. YouTube sees your internet address, as when the child opens youtube.com. No account or key is used.

Everything else stays on the PC: minutes used per day, the names of sites visited (not full addresses) and blocked pages, kept for 30 days and readable by administrators only. The block lists are built into the extension, and pages are checked on the PC, never sent anywhere.

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
