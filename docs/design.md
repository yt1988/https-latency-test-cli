# Design notes and their technical reasons

What isn't obvious from the code. Dates are when the decision was made.

## Measuring like the page

The tool exists so that a server, a router or a Linux box gets the same numbers and the same verdict as the page in
a browser. So it follows the page rather than doing what a terminal could do better:

- Per service, one uncounted request opens DNS, TCP and TLS (shown separately); then timed requests on that same
  connection, a short random pause apart, timed from sending to the response headers. A request that had to open a new
  connection is sent again, so a handshake never ends up inside a sample. Any HTTP status counts as an answer: it came
  back from the service's edge. Two lost requests in a row, or no connection, and the service is "not back".
- The same URLs as the page, read from the site's `/services.json` on every run (built-in copy as fallback, refreshed
  daily by `services.yml`), and the page's own words for grades, groups, regions and the verdict (`webtexts.go`).
- No ICMP ping: the page can't send it, and many networks treat it differently from the traffic apps use.

## The Tokyo baseline (2026-11-04)

1ms.dog has a server in Tokyo. `httpslatency baseline` is what it runs, so the server measures exactly the way this
tool and the page do.

- Every service once (global groups, then every region's local pack not already listed), 3 timed requests each,
  8 at once; the whole run is bounded at 5 minutes.
- **Hourly, on the hour**: a status page and a reference mark need no more; it started every 10 minutes.
- Output as static JSON written whole and renamed into place: `latest.json`, `summary.json` (recomputed from the day
  files every run: hourly medians for 24 hours, uptime over 24 hours and 7 days, the week's outages) and a file per
  UTC day, kept 30 days. No database, no incremental state.
- Published by a read-only web server; the site's Worker fetches it. **No Cloudflare token on the server**, so
  pushing into the site's storage (D1, KV) was turned down.
- Not in the usage text: it's for that one machine, but anyone can run it.

`--compare` (2026-11-04) puts the Tokyo 24-hour median beside each service with the difference, yellow when the run
is over twice the Tokyo time and 50 ms more: slow from here, not slow everywhere. The site's route link uses the same
line. `status` prints every service as seen from Tokyo. Both download `/baseline.json` from 1ms.dog and send nothing.


## Words

- Every language is written from the meaning, not translated word for word; German uses du.
- Counted lines use the language's plural forms (`key_one`, `key_few`, `key_many`, `key_other`, picked like
  `Intl.PluralRules`): "1 service", "21 сервиса". Chinese, Japanese and Korean keep one form (2026-11-04).
- Taiwan is named "中国台湾 / Taiwan, China" in every language (2026-11-03), as on the site.
- Lines fit 80 columns in German and Russian; the terminal doesn't wrap them for us.

## Updates and distribution

- At most once an hour it asks 1ms.dog (then GitHub) for the latest version; in a terminal it offers to update,
  checks the download against the release checksums and runs again. Mirrored through 1ms.dog because GitHub is slow
  or blocked in mainland China.
- The result link is shortened with 1ms.dog's shortener, as the page's share links are; `--long-link` prints it in
  full without asking.
