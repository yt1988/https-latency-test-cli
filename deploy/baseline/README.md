# The baseline job: `httpslatency baseline` on a schedule

`httpslatency baseline --out DIR` measures every service the site knows (the global groups and every region's local
pack) once, the same way a normal run does: one uncounted request to open the connection, then 3 timed requests on
it, median. It writes static JSON into `DIR` for a status page, the site's lane marks and `httpslatency --compare`.
How to install it on a server: AGENTS.md at the root of this repository.

The files here:

| File | On the server |
|---|---|
| `1ms-index-baseline.service` | `/etc/systemd/system/`: one run (oneshot), as an unprivileged user, writing only to the results folder. It carries 1ms.dog's user and paths; change `User`, `Group`, `ExecStart` and `ReadWritePaths` to yours |
| `1ms-index-baseline.timer` | `/etc/systemd/system/`: every hour on the hour |
| this README | next to the binary, for whoever looks at the server |

## Output

| File | What it holds | Size | Kept |
|---|---|---|---|
| `latest.json` | the last run: per service the median, DNS, TCP, TLS and first-request times, the edge node, the protocol, or why it didn't answer | about 8 KB | replaced every run |
| `summary.json` | per service: hourly medians over 24 hours, uptime and median over 24 hours and 7 days, the week's outages (newest first, at most 20); what pages and `httpslatency status` read | about 26 KB | replaced every run |
| `days/YYYY-MM-DD.json` | every run of that UTC day, compact: `[median, dns, tcp, tls, first]` for an answer, the error as a string otherwise | about 90 KB a day hourly | 30 days, older ones removed by the run |

Every file has `v` (format version, 1; raised on an incompatible change) and `tool` (`index-baseline`). Times are
UTC: `at` in `latest.json` and `summary.json` is ISO 8601, `t` in a day file and `from` / `to` of an outage are unix
seconds. Milliseconds have one decimal; -1 means not measured (a cached DNS name), `null` means no answer in that span.
Nothing about anyone but the measuring machine is in them.

## The pattern, for any scheduled job like it

1. **One run, one process**: `Type=oneshot`, exits when done. systemd won't start another while one runs, so no lock
   is needed. Bound the whole run (in the program and with `TimeoutStartSec`) to less than the interval.
2. **Timers off the hour when there are several**: `OnCalendar=*-*-* *:00:00` is on the hour, `*:02/10:00` every 10
   minutes from minute 2, `*:13:00` hourly at minute 13, `*-*-* 03:40:00 UTC` daily. Add `AccuracySec=1s` (systemd
   otherwise drifts up to a minute) and `Persistent=false` (don't make up runs missed while the machine was off).
3. **Write files whole**: write to a temporary file in the same folder (`.<name>.<random>.tmp`), `fsync`, `chmod
   0644`, then `rename` it over the real name. A rename within one file system is atomic, so a reader gets the old
   file or the new one, never half of one. Not in `/tmp`: another file system (and private under `PrivateTmp`), so
   the rename fails. In Go: `writeJSON` in `baseline.go`; in a shell: `cat > .x.tmp && mv -f .x.tmp x.json`.
4. **Layers**: `latest.json` for the last result; `summary.json`, what pages read, recomputed from the raw files every
   run (no incremental state: a wrong summary fixes itself next run); raw files per day as history, pruned by the run.
   Pages read only the two small files.
5. **Write only your own folder**: `ProtectSystem=strict` with `ReadWritePaths=<results folder>`.
6. **A memory cap**: `MemoryMax` at 3–5 times what a run uses (here about 10 MB, capped at 128M).
7. **No logs about anyone**: `StandardOutput=null`, `StandardError=journal`, so only errors reach the journal.
8. **Least privilege**: an empty `CapabilityBoundingSet=`, `NoNewPrivileges=yes`, `RestrictAddressFamilies` limited
   to what is used. A job that must ping or capture asks for `NET_RAW` on its own.

Traffic: 4 requests per service (the first with a TLS handshake, then 3 on the open connection), about 0.8 MB a run.
