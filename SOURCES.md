# Sources, licences and attributions

Built 3 Nov 2026 by `tools/build-lists.mjs`. Downloaded copies live in `.cache/` (git-ignored; hashes in `.cache/manifest.json`).

## Lists inside ext/rules/*.json
| Dataset | Used for | Version or date | Licence | Attribution line |
|---|---|---|---|---|
| The Block List Project, porn and gambling lists (`alt-version/porn-nl.txt`, `gambling-nl.txt`), https://github.com/blocklistproject/Lists, pinned to commit ee3bdbaddf49c756cf3340bb2e6a9dee5282d1c6 | adult.json, gambling.json | file headers dated 2026-07-18 (porn); fetched 3 Nov 2026 | Unlicense (repository LICENSE read; the file headers say MIT, both allow our use) | "Block List Project, https://github.com/blocklistproject/Lists" |
| UT1 blacklists, Universite Toulouse Capitole, https://dsi.ut-capitole.fr/blacklists/ : adult, gambling, redirector (web proxies), vpn | adult.json, gambling.json, bypass.json | archives dated 2 Nov 2026, fetched 3 Nov 2026 | Creative Commons BY-SA 4.0 as published by UT1 (their site showed a maintenance page on 3 Nov 2026, so I could not re-read the licence text: re-check before release). Share-alike applies to the derived rulesets. | "Blacklists UT1 (Universite Toulouse Capitole), CC BY-SA 4.0" |
| EasyList and EasyPrivacy, https://easylist.to (only pure `\|\|domain^` rules) | ads.json | EasyList 202611030947; EasyPrivacy fetched 3 Nov 2026 | dual GPLv3 or CC BY-SA 3.0; we use the CC BY-SA 3.0 option (https://easylist.to/pages/licence.html). Share-alike applies to ads.json. | "The EasyList authors, https://easylist.to/" |
| Majestic Million, https://majestic.com/reports/majestic-million | ranking only (which listed domains are popular); no Majestic data is shipped except through that ordering | fetched 3 Nov 2026 | CC BY 3.0 (as stated by https://tranco-list.eu/ attribution section) | "Data from Majestic Million, https://majestic.com/ (CC BY 3.0)" |
| Hand lists in build-lists.mjs (chat-with-strangers sites, proxies, VPN vendors, social asset domains) | bypass.json, social.json | 3 Nov 2026 | our own | none |

Compared and not used:
- Peter Lowe's ad server list (pgl.yoyo.org): no explicit licence found; skipped.
- StevenBlack/hosts (MIT): its social and gambling extensions are merged lists that already include the Block List Project and ad lists; no extra value, skipped.
- Tranco: its mix includes data under CC BY-NC 4.0, so not used; Majestic alone is CC BY 3.0.
- Block List Project ads list: tried, then dropped (adds publisher and CDN sites such as brightcove.com).

How the size limit is kept (12 MB; result about 6.8 MB): every category domain appears twice (redirect rule for main_frame, block rule for the other types), so a category cap is a domain count: adult 110,000, gambling 35,000, bypass 25,000, ads all (86,256). Choice order inside a category: hand list first, then domains in the Majestic top million that at least two sources list (adult, gambling) or that any source lists (bypass), by rank, then domains listed by two sources, shortest name first, then the rest. The long tail beyond the cap (millions of adult domains) is covered by AdGuard DNS Family in the browser policy. A domain is dropped if a parent domain is already listed (requestDomains matches subdomains). Schools, universities, governments and the military (.edu, .gov, .ac.xx and so on) and a reviewed exclusion list (false positives) are removed. Domains equal to, under, or above a protected domain (essentials and shared/learning-domains.json) are removed, and the build fails if one slips in.

## Curated data
- shared/bookmarks.json: written by hand; every URL fetched with curl on 3 Nov 2026. Site names and links belong to their owners.
- shared/channels.json: written by hand; every channel id checked against the public YouTube RSS feed on 3 Nov 2026. Channel names belong to their owners.
- shared/learning-domains.json and shared/social-domains.json: written by hand from the above.

## Cosmetic ad hiding, YouTube ad rules, curated videos (3 Nov 2026)
- `ext/rules/cosmetic.css` is built by `tools/build-cosmetic.mjs` from EasyList element-hiding rules: `easylist/easylist_general_hide.txt`, `easylist_specific_hide.txt`, `easylist_allowlist_general_hide.txt` and `easylist_allowlist.txt` (for `$generichide` / `$elemhide` sites), pinned to github.com/easylist/easylist commit `7be4364afb7d5b776b2604240a7e9f071f1c1ba4` (2026-11-03 10:37 UTC). Downloads in `.cache/b-easylist_*.txt`.
  - Licence: EasyList is dual-licensed GPLv3+ or CC BY-SA 3.0+ (https://easylist.to/pages/licence.html). We use it under **CC BY-SA 3.0** (https://creativecommons.org/licenses/by-sa/3.0/), with attribution "The EasyList authors (https://easylist.to/)". `cosmetic.css` is a derived work and is shared under the same licence; its header says so. The rest of the extension is not derived from EasyList.
  - Kept: all generic `##` rules, EasyList's per-site exceptions (`#@#`, `~domain`), and site-specific rules for the biggest sites (ranked by the Tranco list, below) while the file stays under 400 KB, plus all of youtube.com. Dropped: extended syntax that is not CSS (`#?#`, `#$#`, `:-abp-*`, `:has-text()` and similar) and 4 selectors Chrome rejects (`node tools/build-cosmetic.mjs --validate` checks every selector in headless Chrome).
- Tranco list `GQ38K` (top 1 million sites, 2026-10-04 to 2026-11-02; https://tranco-list.eu/list/GQ38K): used only at build time to choose which sites get their specific rules; not shipped. Tranco: Le Pochat et al., "Tranco: A Research-Oriented Top Sites Ranking Hardened Against Manipulation", NDSS 2019.
- `ext/rules/yt-ads.json` (22 rules) and `ext/content/yt-ads-main.js`: our own work (YouTube's own ad and ad-measurement endpoints, Google ad servers when loaded from YouTube pages; removal of ad fields from YouTube's player data). No code from uBlock Origin or other GPL tools.
- Curated videos: the public YouTube channel feeds `https://www.youtube.com/feeds/videos.xml?channel_id=<id>` (no API key), for the channels in `shared/channels.json`; thumbnails are loaded from `i.ytimg.com` by the viewer's browser. Nothing is stored beyond a 6-hour cache of titles, ids and dates in the browser.

## Built-in videos and the topic filter (4 Nov 2026, 0.3.1)
- `shared/videos.json` (copied to `ext/data/videos.json`): titles, ids and approximate dates of up to 12 recent videos per curated channel, read from each channel's public Videos tab with yt-dlp 2026.10.21 (https://github.com/yt-dlp/yt-dlp, Unlicense; the release binary `yt-dlp_macos`, checked against the release's SHA2-256SUMS) by `tools/update-videos.mjs`. yt-dlp is a build-time tool only: it is not shipped and no code from it is in the extension. Thumbnails load from `i.ytimg.com` in the viewer's browser, as before. Built 4 Nov 2026: 58 of 59 channels, 676 videos (Hey Duggee has no public Videos tab).
- `shared/topics.json`: our own word lists (en, ka, ru), topic sites and helplines, written by hand; every domain answered on 4 Nov 2026 (some with a bot check). No outside list is used.
- Helplines added 4 Nov 2026 after review: six national child helplines on 116 111 whose sites answered and name the number (116111.pl, kek-vonal.hu, vaikulinija.lt, linkabezpeci.cz, telefonulcopilului.ro, lasteabi.ee). No official website for Georgia's 116 111 could be confirmed; Georgia is reached through findahelpline.com.

## Android app data (5 Nov 2026, P2-162)
| Dataset | Used for | Version or date | Licence | Attribution line |
|---|---|---|---|---|
| HaGeZi DNS blocklists, "Encrypted DNS Bypass": `wildcard/doh-onlydomains.txt` and `ips/doh.txt`, https://github.com/hagezi/dns-blocklists, pinned to commit eb7db8ed27237af2f3c0cb79c9c735e3e076dbad | shared/doh.json (built by tools/build-lists.mjs): 3,588 host names of DoH, DoT and DoQ servers, 1,504 DoH server addresses (IPv4) | file header "Last modified: 04 Nov 2026 16:53 UTC", version 2026.1104.1653.53; fetched 5 Nov 2026 | GPL-3.0 (repository LICENSE read), the same as RaiseSteady Guard; shared/doh.json is shared under it | "HaGeZi DNS blocklists, https://github.com/hagezi/dns-blocklists (GPL-3.0)" |

- shared/doh.json changes against the source: `adguard-dns.com` is left out (a parent of our upstream `family.adguard-dns.com`; the Android filter matches parents), and `use-application-dns.net` is added (Firefox turns its own DoH off when that name has no answer). Helplines, protected domains and their parents are never listed. AdGuard's addresses stay in `ips`: Guard's own lookups do not pass through its VPN.
- shared/android-apps.json: written by hand from pc-apps.json and social-domains.json (same minimum ages); every package id opened on its Google Play page on 5 Nov 2026 (com.ss.android.ugc.trill is listed in Asian stores only: checked with gl=JP).
- The Android app packs ext/rules/{adult,gambling,bypass,social}.json, shared/doh.json and the topic sites of shared/topics.json into `lists.bin` (hashes) at build time (android/build-logic, SharedDataTask); no other data is added.

## Android app code reused (5 Nov 2026, P2-162)
- VPN DNS filter: adapted from AdAway (https://github.com/AdAway/AdAway, GPL-3.0, itself derived from DNS66 and AdBuster), pinned to commit fa16432c3b8f3fa7fa42d76d094b2f285879726f (29 Dec 2025). Files in android/app/src/main/java/com/raisesteady/guard/vpn/adaway/; each keeps its header and lists our changes. Compared and not used (4-5 Nov 2026): DNS66 (archived), Rethink DNS (Apache-2.0 + MPL-2.0, Go core too heavy), personalDNSfilter (GPL-2.0 only, cannot join a GPL-3.0-only project), NetGuard (GPL-3.0, per-app firewall; DNS only is needed).

