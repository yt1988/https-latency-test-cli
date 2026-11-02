# httpslatency

**English** · [简体中文](README.zh-CN.md)

How fast is the real internet from this machine? `httpslatency` times **real HTTPS round trips** to the same 40+ services as [1ms.dog](https://1ms.dog) (ChatGPT, Claude, YouTube, Netflix, GitHub, Docker Hub, Steam, the big CDNs …) plus a pack of local favorites for the country it runs from. Not ping, not a TCP handshake: an HTTPS request to each service's own host, answered by its edge, over a warm connection, the way a browser sees it. One binary, no dependencies; for VPSes, Linux boxes, routers (MIPS/ARM) and desktops.

```sh
curl -fsSL https://1ms.dog/i | sh
```

That installs `httpslatency` to `/usr/local/bin` (or `~/.local/bin`) and starts it with a short menu: everything, some groups, or another region's local favorites; IPv4 or IPv6; how many timed requests; and whether to save the result. It then prints the same test as a command, to run again without the menu. On Windows, in PowerShell:

```powershell
irm https://1ms.dog/i.ps1 | iex
```

Other ways:

```sh
curl -fsSL https://1ms.dog/i | sh -s -- run          # run once from a temp dir, install nothing
curl -fsSL https://1ms.dog/i | sh -s -- --no-run     # install only
httpslatency -c ai,video,game                       # some groups
httpslatency -s chatgpt,claude,github -n 10          # some services, 10 timed requests each
httpslatency -P cn                                   # with mainland China's local favorites
httpslatency -6 -v                                   # over IPv6, with a detail line per service
httpslatency -o md                                   # also save the result as Markdown (or -o json)
httpslatency --json > result.json
```

The installer downloads from 1ms.dog (which passes GitHub's release files on, for places where GitHub is slow) or GitHub, and checks the SHA-256 against the release's `checksums.txt`. Builds: Linux (amd64, arm64, armv7, 386, riscv64, mips, mipsle), macOS, FreeBSD, Windows ([releases](https://github.com/yt1988/https-latency-test-cli/releases)). With Go: `go install github.com/yt1988/https-latency-test-cli@latest`.

## What it shows

```
HTTPS latency to 44 services · 5 timed requests each
from Tokyo, JP · AS2516 KDDI CORPORATION

  Local favorites · Japan
    Yahoo! JAPAN         9.8 ms  ━━━●················  Zoomies
    LINE                  12 ms  ━━━━●···············  Zoomies    NRT
  AI
    ChatGPT              5.1 ms  ━●··················  Zoomies    NRT
    Claude               5.4 ms  ━●··················  Zoomies    NRT
  …
› Local favorites 11 ms  ·  Global services 38 ms  ·  overall median 31 ms
› Fast nearby and worldwide — Gaming, calls and streaming should all feel smooth from here.
  Gaming smooth  ·  Video calls smooth  ·  Streaming smooth  ·  AI tools smooth  ·  Dev tools smooth
› Result link: https://1ms.dog/r/2.…
```

Each row is a service's median in ms, on the same log-scale lane as the page's field (5 ms to 1 s), with the page's dog-speed grade and the edge node that answered (an airport code, where the service names it in its headers). The verdict is the page's: local favorites against global services, then gaming, calls, streaming, AI and developer tools. `-v` adds a line per service: address, HTTP version and status, the first request's DNS, TCP and TLS times, every sample, and jitter.

The **result link** opens the run on 1ms.dog, like a result shared from the page. It holds the country this ran from and each service's time, never an address; the tool only prints it. `--no-link` leaves it out.

## How it measures

- **The page's method, outside a browser.** Per service, one uncounted request opens DNS, TCP and TLS; each timed request then goes over that same connection, 120–200 ms apart, timed from sending it to having the response headers (what a browser's fetch waits for). HTTP/2 where the service offers it. A random query string skips caches, except for the CDN rows, which would go back to their origin with one.
- **Any HTTP answer counts**, an error status too: it still came back from the service's edge. A timed request that had to open a new connection (the server closed the old one) isn't counted and is sent again, so a handshake never lands in a sample.
- **Not back** means no connection within 5 seconds, or two timed requests in a row lost (4 s each, `-t`).
- **Same list as the page.** Each run reads [1ms.dog/services.json](https://1ms.dog/services.json), which the site builds from its own list; a copy is built in for when it can't be reached (`--offline`), and a daily workflow keeps it current. The local pack follows the country this runs from (`-P` picks another), and Telegram is measured at the data center most accounts in that region use, as on the page.
- Eight services at a time (`-C`), all with their own connection. Direct: proxy variables are not used, since the point is this machine's own route.

## Options

`httpslatency -h` lists them all: `-s` services, `-c` groups (`local`, `ai`, `web`, `video`, `social`, `dev`, `game`, `cdn`), `-P` local pack, `-4`/`-6`, `-n` timed requests, `-t` timeout, `-C` concurrency, `-o md|json` and `--file` to save, `--json` for stdout, `-v`, `--no-link`, `--lang` (10 languages; the menu asks once and remembers it), `--offline`, `--services FILE`, `--list`, `--update`.

**Updates.** At most once an hour httpslatency asks 1ms.dog for the latest release; in a terminal it offers to update, checks the new binary against the release checksums, swaps it in and runs again. `httpslatency --update` does it right away; `--offline` or `HTTPSLATENCY_NO_UPDATE=1` turns it off.

## Privacy

Each run fetches the current service list from 1ms.dog and asks it where the run comes from (`1ms.dog/_ms/me`, with Cloudflare's `speed.cloudflare.com/meta` as the fallback), for the "from" line and the local pack: city, country and network. The address itself is never shown or saved. At most once an hour it asks 1ms.dog for the latest version. `--offline` skips all three. Nothing else is sent anywhere but the requests to the services themselves, and results are never uploaded.

## License

MIT
