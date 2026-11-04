# 1ms-index-baseline：东京基线（以及其他定时任务照抄的写法）

1ms.dog 的东京服务器每小时（整点）跑一次 `httpslatency baseline`，从东京测一遍首页知道的全部服务（全球分组加上各地本地包，约 84 个），用的方法和普通运行一样：先发一个不计时的请求建立连接，再在这条连接上计时 3 次，取中位数。结果写成静态 JSON，经 `https://data.1ms.dog:2083/index/` 只读发布，由首页的 Worker 拉取并缓存。服务器停了，网站只是数据不再更新。

本目录的文件在服务器上的位置：

| 文件 | 服务器上的位置 |
|---|---|
| `httpslatency`（linux 二进制，在别处编译好再上传） | `/opt/1ms/index/httpslatency` |
| `1ms-index-baseline.service` | `/etc/systemd/system/` |
| `1ms-index-baseline.timer` | `/etc/systemd/system/` |
| 本 README | `/opt/1ms/index/README.md` |

## 发布的文件

都在 `/srv/1ms-data/index/`，对外是 `https://data.1ms.dog:2083/index/<文件>`。

| 文件 | 内容 | 大小 | 保留 |
|---|---|---|---|
| `latest.json` | 最近一次运行：每个服务的中位数、DNS、TCP、TLS、首个请求耗时、边缘节点、协议；没应答的写原因 | 约 8 KB | 每次覆盖 |
| `summary.json` | 每个服务：24 小时内逐小时的中位数、24 小时和 7 天的可用率与中位数、7 天内的故障（最新的在前，最多 20 条）；首页曲线、`/status` 页和 `httpslatency status` 读的是它 | 约 26 KB | 每次覆盖 |
| `days/YYYY-MM-DD.json` | 当天（UTC）的每一次运行，紧凑格式：答上的写 `[中位数, dns, tcp, tls, 首个请求]`，没答上的写错误字符串 | 每天约 90 KB | 30 天，运行时自动删除更早的 |

所有文件的公共字段：`v`（格式版本，目前是 1，改动不兼容时加 1）、`tool`（`index-baseline`），时间统一用 UTC。`latest.json` 和 `summary.json` 里的 `at` 是 ISO 8601 格式，day 文件里的 `t` 和故障区间的 `from`、`to` 是 unix 秒。毫秒值保留 1 位小数；-1 表示没测到（例如 DNS 命中了缓存），`null` 表示那段时间里没有一次应答。

文件里只有这台机器自己测到的数据，没有任何访客信息。

## 其他定时任务照抄的写法

dns-watch、cdn-watch、me2cloud-watch、unlock-watch、asn-history 都按下面这套做：

1. **一次运行就是一个进程**：`Type=oneshot`，跑完就退出，不常驻。上一次没跑完时，timer 不会再起一个新的，所以不需要加锁。给整次运行设一个总超时（程序里一个，unit 的 `TimeoutStartSec` 一个），时长要短于运行间隔。
2. **timer 错开整点**：首页是每小时整点 `OnCalendar=*-*-* *:00:00`；每 10 分钟、从第 2 分开始写 `*:02/10:00`，每小时第 13 分写 `*:13:00`，每天 03:40 UTC 写 `*-*-* 03:40:00 UTC`。加上 `AccuracySec=1s`（否则 systemd 默认会有最多 1 分钟的漂移），再加 `Persistent=false`（停机期间错过的不补跑）。
3. **原子写文件**：先写到同一目录下的临时文件（`.<名字>.<随机>.tmp`），`fsync`，`chmod 0644`，再 `rename` 成正式名字。同一个文件系统内的 rename 是原子的，Caddy 读到的要么是旧文件、要么是新文件，不会读到写了一半的。临时文件不能放 `/tmp`：它和结果目录不在同一个文件系统，rename 会失败；何况 `PrivateTmp` 下 `/tmp` 也不是同一个。Go 的写法见 `baseline.go` 的 `writeJSON`；shell 的写法是 `cat > .x.tmp && mv -f .x.tmp x.json`。
4. **分层文件**：`latest.json` 放最近一次结果；`summary.json` 放网页直接用的汇总，每次运行时从原始数据重新算一遍（不做增量，算错了下一次就自愈）；按天分的原始文件做历史，在运行时删掉超出保留期的。网页只读前两个小文件，不读原始文件。
5. **只写自己的结果目录**：`ProtectSystem=strict` 加上 `ReadWritePaths=/srv/1ms-data/<工具 id>`，其他位置全部只读。
6. **资源上限**：设 `MemoryMax`，按实测的 3～5 倍定（这里实测约 20 MB，上限 128M）。整台机器只有 939 MB 内存。
7. **不记日志**：`StandardOutput=null`、`StandardError=journal`，只有出错才进 journal。
8. **最小权限**：`CapabilityBoundingSet=` 为空、`NoNewPrivileges=yes`、`RestrictAddressFamilies` 只留要用的协议族。确实要 ping 或抓包的，单独申请 `NET_RAW`。
9. **格式**：JSON，带 `v`、`tool`，时间用 UTC。

## 部署和验证

```
# 上传二进制（GOOS=linux，GOARCH 按服务器实际架构）后：
chown root:root /opt/1ms/index/httpslatency && chmod 755 /opt/1ms/index/httpslatency
systemctl daemon-reload
systemctl start 1ms-index-baseline.service     # 先手动跑一次
systemctl status 1ms-index-baseline.service    # 应该是 inactive (dead)、status=0/SUCCESS
systemctl enable --now 1ms-index-baseline.timer
systemctl list-timers 1ms-index-baseline.timer # 下一次在整点
ls -la /srv/1ms-data/index /srv/1ms-data/index/days
```

从外部验证：`curl -s https://data.1ms.dog:2083/index/summary.json | head -c 300`。

流量：每个服务 4 个请求（首个请求带 TLS 握手，后面 3 个复用连接），整次运行约 0.8 MB，每月约 0.6 GB。
