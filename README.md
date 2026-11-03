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
› Result link: https://s.1ms.dog/TuuUvj
```

Each row is a service's median in ms, on the same log-scale lane as the page's field (5 ms to 1 s), with the page's dog-speed grade and the edge node that answered (an airport code, where the service names it in its headers). The verdict is the page's: local favorites against global services, then gaming, calls, streaming, AI and developer tools. `-v` adds a line per service: address, HTTP version and status, the first request's DNS, TCP and TLS times, every sample, and jitter.

The **result link** opens the run on 1ms.dog, like a result shared from the page. It holds the country this ran from and each service's time, never an address, and is shortened with 1ms.dog's link shortener, as the page's share links are (`s.1ms.dog/…`). `--long-link` prints it in full without asking the shortener; `--no-link` leaves it out.

## How it measures

- **The page's method, outside a browser.** Per service, one uncounted request opens DNS, TCP and TLS; each timed request then goes over that same connection, 120–200 ms apart, timed from sending it to having the response headers (what a browser's fetch waits for). HTTP/2 where the service offers it. A random query string skips caches, except for the CDN rows, which would go back to their origin with one.
- **Any HTTP answer counts**, an error status too: it still came back from the service's edge. A timed request that had to open a new connection (the server closed the old one) isn't counted and is sent again, so a handshake never lands in a sample.
- **Not back** means no connection within 5 seconds, or two timed requests in a row lost (4 s each, `-t`).
- **Same list as the page.** Each run reads [1ms.dog/services.json](https://1ms.dog/services.json), which the site builds from its own list; a copy is built in for when it can't be reached (`--offline`), and a daily workflow keeps it current. The local pack follows the country this runs from (`-P` picks another), and Telegram is measured at the data center most accounts in that region use, as on the page.
- Eight services at a time (`-C`), all with their own connection. Direct: proxy variables are not used, since the point is this machine's own route.

## Options

`httpslatency -h` lists them all: `-s` services, `-c` groups (`local`, `ai`, `web`, `video`, `social`, `dev`, `game`, `cdn`), `-P` local pack, `-4`/`-6`, `-n` timed requests, `-t` timeout, `-C` concurrency, `-o md|json` and `--file` to save, `--json` for stdout, `-v`, `--no-link`, `--long-link`, `--lang` (10 languages; the menu asks once and remembers it), `--offline`, `--services FILE`, `--list`, `--update`.

**Updates.** At most once an hour httpslatency asks 1ms.dog for the latest release; in a terminal it offers to update, checks the new binary against the release checksums, swaps it in and runs again. `httpslatency --update` does it right away; `--offline` or `HTTPSLATENCY_NO_UPDATE=1` turns it off.

## Privacy

Each run fetches the current service list from 1ms.dog and asks it where the run comes from (`1ms.dog/_ms/me`, with Cloudflare's `speed.cloudflare.com/meta` as the fallback), for the "from" line and the local pack: city, country and network. The address itself is never shown or saved. The result link (country and times only) goes to 1ms.dog's link shortener to be shortened; `--long-link` skips that. At most once an hour it asks 1ms.dog for the latest version. `--offline` skips all of these. Nothing else is sent anywhere but the requests to the services themselves, and the full results are never uploaded.

<!-- 1ms-repos: generated by 1ms-shell scripts/repos.mjs; edit the list there, not here -->
## More from 1ms.dog

Free, open-source network tools from [1ms.dog](https://1ms.dog).

**In your browser**

| Tool | What it answers | Source |
|---|---|---|
| [1ms.dog](https://1ms.dog) | Where your traffic exits, and how far the services you use are | [1ms.dog](https://github.com/yt1988/1ms.dog) |
| [Who Are You](https://who.1ms.dog) | What any website can tell about you without asking | [who-are-you](https://github.com/yt1988/who-are-you) |
| [EgressLens](https://egress.1ms.dog) | Every exit IP your traffic uses, and which sites see which | [egresslens](https://github.com/yt1988/egresslens) |
| [IPQS](https://ipqs.1ms.dog) | How clean your IP is, scored 0–100 from a dozen sources | [ip-quality-test](https://github.com/yt1988/ip-quality-test) |
| [Streaming & AI Unlock Check](https://unlock.1ms.dog) | Which streaming and AI services work on your IP | [unlock-check](https://github.com/yt1988/unlock-check) |
| [DNS Test](https://dns.1ms.dog) | Which public DNS is fastest from your network | [dns-test](https://github.com/yt1988/dns-test) |
| [IPv6 Test](https://ipv6.1ms.dog) | Whether your network really supports IPv6, scored out of 10 | [ipv6-test](https://github.com/yt1988/ipv6-test) |
| [Speed Test](https://speed.1ms.dog) | Download, upload, ping and bufferbloat | [speed-test](https://github.com/yt1988/speed-test) |
| [CDN Radar](https://cdn.1ms.dog) | Which location of each big CDN serves you, and how far it is | [cdn-radar](https://github.com/yt1988/cdn-radar) |
| [Me2Cloud](https://m2c.1ms.dog) | Latency, jitter and loss to every region of the big clouds | [me2cloud](https://github.com/yt1988/me2cloud) |
| [Network X-Ray](https://check.1ms.dog) | Eight of these tests in one run, one grade | [connection-checkup](https://github.com/yt1988/connection-checkup) |
| [ASN Lookup](https://asn.1ms.dog) | Who owns an IP, and the prefixes and peers of every AS | [asn-lookup](https://github.com/yt1988/asn-lookup) |
| [Cloud Cost Planner](https://cost.1ms.dog) | The cheapest VPS, storage and CDN setup for your traffic | [cloud-cost-planner](https://github.com/yt1988/cloud-cost-planner) |
| [AWS Lightsail Cost Saver](https://ls.1ms.dog) | Forecast Lightsail transfer overage, and the cheapest way to add more | [lightsail-cost-saver](https://github.com/yt1988/lightsail-cost-saver) |

**On your server**

| Tool | What it answers | Source |
|---|---|---|
| [`dnstest`](https://github.com/yt1988/dns-test-cli) | DNS Test from a server, over UDP, TCP, DoT, DoH and DoQ | [dns-test-cli](https://github.com/yt1988/dns-test-cli) |
| [`vps2cloud`](https://github.com/yt1988/vps2cloud) | Me2Cloud from a server: latency to every region of the big clouds | [vps2cloud](https://github.com/yt1988/vps2cloud) |
<!-- /1ms-repos -->

## License

MIT
