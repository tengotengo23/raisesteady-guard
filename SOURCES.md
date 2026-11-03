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
