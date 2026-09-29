# Ignite — Whole-Codebase Code Review

**Date:** 2026-09-29 · **Scope:** entire Go codebase (~18,000 lines, 14 packages: `main`, `app`, `config`, `db`, `dhcp`, `handlers`, `routes`, `tftp`, `ipxe`, `syslinux`, `osimage`, `interfaces`, `vmtest`, `testdata`)
**Focus:** structure, data types, bugs · **Method:** static code reading (no Go toolchain was available in the review environment, so nothing here was compiler- or `go vet`-verified — treat line numbers as of commit `3b4c996`)

## TL;DR — fix these first

1. **Authentication is a no-op.** `isAuthenticated` returns true for *any* non-empty `ignite_session` cookie (`handlers/auth.go:174`). Every "protected" route — file delete, DHCP control, IPMI with BMC credentials — is effectively public. Combined with predictable session tokens and hardcoded `admin`/`admin`, this is the single worst issue in the codebase.
2. **Unauthenticated remote file read / write / delete** in at least five independent places: TFTP `readHandler`/`writeHandler` (`tftp/tftp.go:64,77`), provision `LoadFileContent`/`SaveFileContent` (`handlers/provision.go:443-501`), TFTP `ViewFile`/`HandleDelete` (`handlers/tftp.go:131,168`), and the legacy provision template ops (`handlers/provision.go:200+`). Because of #1, none of these require login.
3. **DHCP is broken at the core.** Three independent criticals: an unguarded `handlers` map that will panic the process under concurrent admin use (`dhcp/service.go:16`), the REQUEST path persisting every lease under the empty-string key so only one lease can ever exist (`dhcp/protocol_handler.go:162`), and unguarded check-then-act IP allocation in two duplicated implementations.
4. **Download state machines race.** Both `osimage` and `syslinux` cancellation can be silently overwritten; a failed syslinux download deletes the working boot files *before* the new download succeeds, bricking PXE.
5. **Dead architecture.** The entire `interfaces/` package (~300 lines + ~1200 lines of tests) is imported by nothing and implemented by no one; `syslinux` defines nine helper interfaces with zero implementations. Delete or adopt.

---

## Critical

### CR-1 — Authentication bypass: any cookie value passes
`handlers/auth.go:174`
```go
return cookie.Value != "" && len(cookie.Value) > 0
```
`isAuthenticated` never validates the token against anything server-side. Setting `Cookie: ignite_session=x` bypasses `AuthMiddleware` on every route. This compounds with:
- predictable token format (`auth.go:163`: `username + "_" + time.Now().Format("20060102150405")`),
- hardcoded `admin`/`admin` default credentials (`auth.go:53-54`),
- `ChangePassword` accepting empty passwords and a package-level `defaultPassword` mutated without a mutex (data race under concurrent requests, `auth.go:151`),
- session cookie missing `Secure`/`SameSite` flags (`auth.go:76-84`),
- API/HTMX clients getting a 302 to `/login` instead of 401 (`auth.go:181-208`).

**Fix:** server-side session store (map + mutex, or reuse bbolt), random tokens (`crypto/rand`), require password change on first boot, `HttpOnly`+`Secure`+`SameSite` cookies, 401 for API paths.

### CR-2 — Unauthenticated arbitrary file read (provision API)
`handlers/provision.go:443-458` (`LoadFileContent`)
```go
filePath := r.URL.Query().Get("path")
content, err := h.loadFileContent(filePath)
```
`path` is used verbatim — `?path=/etc/shadow` reads the whole server filesystem. The `isPathAllowed` helper exists but is only applied to `DeleteFile`, not here.

### CR-3 — Unauthenticated arbitrary file write (provision API)
`handlers/provision.go:461-501` (`SaveFileContent`)
```go
filePath := r.FormValue("path")
os.MkdirAll(filepath.Dir(filePath), 0755)
os.WriteFile(filePath, []byte(content), 0644)
```
No path validation at all — remote file write anywhere the process can write.

### CR-4 — Arbitrary file read via TFTP web UI
`handlers/tftp.go:131` (`ViewFile`), `handlers/common.go:460` (`NewViewModal`)
```go
filePath := filepath.Join(TFTPDir, fileName) // fileName = r.URL.Query().Get("file")
```
No validation — `/tftp/view?file=../../etc/passwd` is served as `text/plain`. The well-written `TFTPSecurityValidator` in `security.go` exists but is not called here (it's only used by `HandleDownload` — which is itself broken, see M-16).

### CR-5 — Arbitrary file delete via TFTP web UI
`handlers/tftp.go:168-176` (`HandleDelete`): same unvalidated `?file=` parameter fed to `os.Remove`. `os.Remove` on a directory path also removes empty dirs.

### CR-6 — Path traversal across all legacy provision file ops + panic-DoS in `SubmitBootMenu`
`handlers/provision.go:200` (`LoadTemplate`), `:254` (`LoadConfig`), `:294` (`HandleNewTemplate`), `:335` (`HandleSave`), `:177` (`HandleFileOptions`):
```go
filePath := filepath.Join(provisionDir, "templates", templateType, templateName)
```
`typeSelect`/`templateSelect`/`filename`/`category` come straight from form values — `typeSelect=../../../..` escapes the provision dir for reads and writes alike. Separately, `handlers/bootmenu.go:75-97` builds `configs/<typeSelect>/<mac>` and `templates/<typeSelect>/<template_name>` from user input (only `:` is stripped from `mac`, so `..`/`/` survive → arbitrary write via `os.Create`), then feeds a user-influenced path to `template.Must(template.ParseFiles(...))` (`bootmenu.go:276`) — pointing it at any non-template file makes `ParseFiles` error and `Must` **panic, crashing the whole server**.

### CR-7 — TFTP path traversal + unauthenticated remote file write
`tftp/tftp.go:64,77`
```go
file, err := os.Open(s.serveDir + "/" + filename)   // readHandler
file, err := os.Create(s.serveDir + "/" + filename) // writeHandler
```
No sanitization of the TFTP filename. A client can read `../../etc/cron.d/x`, and `writeHandler` honors TFTP WRQ with `os.Create` — a remote file-write primitive with no auth, no allowlist, no `filepath.Clean` containment check. Zero test coverage of either handler (`tftp_test.go` tests `os.WriteFile`, i.e. the stdlib).

### CR-8 — Concurrent map access in `DHCPServerService` (fatal runtime panic)
`dhcp/service.go:16`
```go
handlers   map[string]*ProtocolHandler
```
Written at `StartServer` (`:136`), read/deleted at `StopServer`/`DeleteServer` (`:164,:185`) — all invoked from HTTP handlers on concurrent goroutines, with no mutex. Concurrent map read/write is a **fatal, unrecoverable** runtime panic in Go: one admin clicking Start while another clicks Stop kills the process. Fix with `sync.RWMutex` or `sync.Map`.

### CR-9 — DHCP REQUEST path stores every lease under the `""` key
`dhcp/protocol_handler.go:162`
```go
newLease := &Lease{
    IP:       requestedIP,
    MAC:      mac,
    Expiry:   time.Now().Add(h.server.LeaseDuration),
    Reserved: false,
    ServerID: h.server.ID,
}
```
No `ID` is set (compare `AssignLease`, which does `ID: uuid.New().String()`). `BoltLeaseRepository.Save` persists with `lease.ID` as the bucket key (`dhcp/repository.go:84`), so **every lease ACKed through the real DHCP packet path overwrites the previous one** — in practice only one DHCP lease can ever exist, the second client to complete DORA wipes the first, and `isIPAvailable` then re-offers the stolen IP. Also missing vs. the service path: `State`, `StateUpdatedAt`, `LastSeen`, `StateHistory`.

### CR-10 — Check-then-act races on IP allocation (two duplicated implementations)
`dhcp/lease_service.go` (`AssignLease` → `isIPAvailable`/`findAvailableIP`) and `dhcp/protocol_handler.go` (`handleDiscover`/`handleRequest`) both do: scan all leases → pick a free IP → save, with no lock and no transaction. Two concurrent DISCOVERs (krolaw's `d4.Serve` handles packets concurrently; HTTP `ReserveLease` races packet handling too) can both observe the same IP as free and persist leases for it. For a DHCP server this is the core correctness property.

---

## Major — Security

### M-S1 — SSRF + disabled TLS verification in IPMI
`handlers/ipmi.go:56-59`: `ip` is a raw form value formatted into `https://%s/redfish/v1` with `Insecure: true` — the server opens HTTPS to any attacker-supplied host, sending submitted BMC credentials with cert verification disabled (MITM-able credential leak). Validate as IP/hostname against an allowlist; never ship `Insecure: true` as a default.

### M-S2 — Reflected XSS in `HandleNewTemplate`
`handlers/provision.go:294-330` interpolates `filename` unescaped into a `Content-Type: application/javascript` response (`fmt.Fprintf(w, "...textContent = 'Filename: %s';...", filename)`). `';alert(1);//` breaks out.

### M-S3 — Response header injection via filenames
`handlers/tftp.go:157` (`HandleDownload`), `:204` (`ViewFile`): `filepath.Base` doesn't strip quotes — `fileName = 'x";\r\nEvil: 1'` injects response headers.

### M-S4 — Upload path traversal, no size enforcement
`handlers/tftp.go:211-222` (`HandleUpload`): `handler.Filename` (multipart, client-controlled, may contain `../`) used unsanitized in `os.Create`; `ValidateTFTPUpload` in `security.go` is never called. `ParseMultipartForm(32<<20)` caps only memory — excess spools to disk unbounded (disk-fill DoS). Add `http.MaxBytesReader`.

### M-S5 — Directory traversal in TFTP browser
`handlers/tftp.go:33-48` → `getFileInfo` (`:246`): `?dir=../../..` passed to `os.ReadDir(filepath.Join(TFTPDir, dir))` lists arbitrary server directories.

---

## Major — Correctness / data integrity

### M-C1 — `osimage.CancelDownload` loses to a stale-write race
`osimage/service.go` (`processDownload`) re-fetches status into a *shadowing* variable used only for the check, then keeps using the stale outer `status`: worker `Save`s `Progress=60` → **overwrites the user's `"cancelled"` row** with stale `"downloading"` → the re-fetch sees `"downloading"` → download runs to `"completed"`. The same struct is also returned to the HTTP handler while the worker mutates it.

### M-C2 — `syslinux.CancelDownload` is cosmetic + data race
`syslinux/service.go:570` only flips the DB row to `"cancelled"`; the `DownloadVersion` goroutine (`:261`) never polls it, keeps downloading/extracting/installing, and its final unconditional `SaveDownloadStatus` overwrites the row with `"completed"`. The `status` pointer is shared between HTTP caller and goroutine with no mutex.

### M-C3 — Failed syslinux download bricks PXE boot
`DownloadVersion`'s goroutine runs `cleanupActiveVersion` (`syslinux/service.js:672` → `service.go:672`) **first** — deleting the currently-serving BIOS+EFI boot files and marking versions not-downloaded — *before* the new download succeeds. Any mid-download network failure leaves the server with no boot files. Download-then-atomic-swap is the safe order.

### M-C4 — Partial/corrupt downloads left in the TFTP serve dir; no timeout
`osimage/service.go:327` (`downloadFile`): `os.Create` + `io.Copy`; on copy failure the truncated file stays and PXE clients can fetch a half-written kernel. `http.Get` with **no timeout/context** — a stalled peer hangs the single worker goroutine forever, blocking the whole download queue. (Same worker-lifecycle gaps: `downloadChan` never closed, no `Stop`, duplicate `DownloadOSImage` calls for the same OS/version can pass the check-then-queue race.)

### M-C5 — Checksum computed but never verified
`osimage/service.go:309` stores `Checksum: kernelChecksum + ":" + initrdChecksum`, but nothing compares against an expected value (config carries none). The model comment claims "SHA256 verification" — it's fingerprinting, not verification.

### M-C6 — Progress callback does a bbolt write per `Read()`
`syslinux/service.go:386` (`progressReader.onProgress`) calls `SaveDownloadStatus` on every read syscall — thousands of serialized bbolt write transactions/sec, stalling the single-writer DB for everything else.

### M-C7 — `InstallBootFiles` is a no-op stub reporting success
`syslinux/service.go:604` returns `nil` unconditionally; `DownloadVersion` gates `"completed"` on it. (Files happen to land via `extractSingleFile` during extraction, so it works by accident — but the interface method lies.)

### M-C8 — `DeleteAllKV` mutates a bucket inside `ForEach`
`db/bolt.go:131`: bbolt docs say the `ForEach` callback "must not modify the bucket; this will result in undefined behavior" (cursor invalidation → skipped keys/panic). The `-clear-data` CLI path relies on this, so wipes can silently leave rows behind. Collect keys first or recreate the bucket.

### M-C9 — `GenericRepository.GetAll` silently drops corrupt entities
`db/generic_repository.go:65`: unmarshal failure → log line + `continue`. Caller sees success; machines silently vanish from the UI. Return the error.

### M-C10 — Untyped "not found" swallows real DB failures
`db/generic_repository.go:44` returns `fmt.Errorf("entity not found")` — no sentinel, so `dhcp/service.go:33` (`CreateServer`) misreads any BoltDB error (locked DB, I/O) as "no such server" and proceeds, risking duplicates. Needs a typed `ErrNotFound` + `errors.Is`.

### M-C11 / M-C12 — 4-byte vs 16-byte `net.IP` confusion
`dhcp/models.go:181` (`IsInRange`): `len(s.IPStart) != len(ip)` → false. IPv4 has two valid `net.IP` forms (4-byte and 16-byte via `net.ParseIP`; JSON round-trip through bbolt always yields 16-byte) — mixing them NAKs in-range clients. And `dhcp/protocol_handler.go:208-211` builds DHCP options with `[]byte(h.server.IP)` — after a DB reload those are 16-byte, but DHCP options require exactly 4. Normalize with `To4()` at the boundary.

### M-C13 — Full config reload on every DHCP packet
`dhcp/protocol_handler.go:190`: `cfg, _ := config.LoadDefault()` per DISCOVER/REQUEST — rebuilds the whole default config including the hundreds-of-lines OS image catalog, re-reads env, discards the error. The inline comment admits it should be injected; `NewProtocolHandler(server, leaseRepo)` takes no config.

### M-C14 — `AssignLease` ignores server ownership
`dhcp/lease_service.go:34`: `GetByMAC` with no `ServerID` check — a MAC leased on server A that DISCOVERs on server B gets server A's lease/IP extended for server B. (The packet path checks `ServerID`; the service path doesn't.)

### M-C15 — Expired/duplicate MAC rows never cleaned
`dhcp/lease_service.go:36-48`: expired lease → falls through and creates a new row without deleting the old; `GetByMAC` iterates a Go map in random order and may return the dead one. Same shape in `handleRequest` when `requestedIP` differs.

### M-C16 — Syslinux HTTP endpoints that lie
`handlers/syslinux.go:270-285` (`DeleteVersion` returns `{"success":true}` without deleting); `:549-571` (`activateVersion` mutates in-memory structs, comment admits persistence needs a `SaveVersion` that doesn't exist — activation never persists); `GetBootFile`/`ListDownloadStatuses` return canned stubs. The UI reports success for no-ops.

### M-C17 — UDP "status checks" always report running
`handlers/status.go:138,183`: `net.DialTimeout("udp", ...)` never handshakes — succeeds with nothing listening. TFTP/DHCP shown "running" when down.

### M-C18 — `lease_time` validated then ignored; silent mask fallback
`handlers/dhcp.go:250,608-617`: `SubmitDHCPServer` validates `lease_time` then hardcodes `LeaseDuration: 2 * time.Hour`. `getMaskBits` returns `"24"` on any invalid mask (and counts bits without checking contiguity — `255.0.255.0` passes as /16), so a typo'd mask silently misconfigures the server.

### M-C19 — Duplicate iPXE menu labels
`ipxe/service.go:67`: `ID: strings.ToLower(img.OS)` — two Ubuntu versions both generate `:ubuntu`; `goto ${selected}` jumps to the first, so the second is unreachable. (Also: `getDisplayName` `:128` indexes `os[0]` — panics on empty OS string; `getKernelArgs` `:152,154` hardcodes `:8080` while `GenerateConfig` uses `s.config.HTTP.Port`; `HTTPPort` is typed `string`.)

### M-C20 — ACK sent even when lease `Save` fails
`dhcp/protocol_handler.go` (`handleRequest`): on `Save` error the code logs and **still sends the ACK** — the client believes it holds a lease the server never persisted.

### M-C21 — Double `Start()` leaks listener + goroutine permanently
`dhcp/protocol_handler.go:39-70`: `Start()` overwrites `h.ctx`/`h.cancel` before `ListenUDP` fails on the bound port — the first handler's cancel func is lost, so the original listener can never be stopped.

---

## Major — Structure & data types

### M-T1 — The `interfaces/` package is dead code
`interfaces/service.go` defines 10 interfaces; **zero** imports of `ignite/interfaces` in production code, **zero** implementations anywhere — while `osimage`/`syslinux` define parallel interfaces that are actually wired. ~300 lines of aspirational API plus ~1200 lines of tests exercising a hand-written mock (tautological). Delete it or adopt it; don't ship both.

### M-T2 — Duplicated domain types
`osimage.DownloadStatus` (`osimage/models.go:26`) ≈ `syslinux.DownloadStatus` (`syslinux/models.go:42`); `syslinux.FileInfo` vs `interfaces.FileInfo` (same name, different shapes); `osimage.OSImageService` vs `interfaces.OSImageService` (same name, different method sets). Unify behind one download-status type.

### M-T3 — Nine syslinux helper interfaces, zero implementations
`syslinux/interfaces.go` (`Downloader`, `Extractor`, `MirrorScanner`, `FileManager`, `EventHandler`, `Logger`, `ConfigProvider`, `CacheManager`, `HTTPClient`) — none implemented or referenced outside the file; the concrete `service` does its own HTTP/tar/file ops inline. Interface bloat that misleads about the design.

### M-T4 — Stringly-typed API surface
`map[string]interface{}` for configs/metrics/events in `interfaces/service.go`; bare `Status string` with magic values (`"queued"`, `"downloading"`, …) in both download packages — no constants, no typed enum, typos fail silently. `dhcp` lease states are untyped string constants on a `string` field — should be `type LeaseState string`. `UpdateLeaseState` (`handlers/dhcp.go`) accepts any string with no allowlist.

### M-T5 — `Unsubscribe()` cannot unsubscribe
`interfaces/service.go:88,120`: no subscription handle/token argument, so no implementation could know which subscriber to remove — subscriber leaks by design.

### M-T6 — Production CLI lives in a directory named `testdata`
`main.go:15` imports `ignite/testdata` for flag parsing, mock seeding, and DB wiping. The go tool **ignores `testdata` dirs** during `./...` expansion, so `go vet ./...` / `go test ./...` / coverage silently skip production code. Rename to `cli`. Related: `-clear-data` irreversibly wipes the DB with no confirmation (`testdata/cli.go:41`).

### M-T7 — `BoltDB` embeds `*bolt.DB`, leaking the whole bbolt API
`db/bolt.go:15` + `GetDB()` (`:49`) promote every bbolt method to all holders, defeating the `Database` interface — and `app/container.go:48` exploits it (`syslinux.NewBoltRepository(database.GetDB())`) while `dhcp`/`osimage` go through the interface. Pick one layering and enforce it.

### M-T8 — Two parallel DI containers, hand-copied
`app/container.go` (`app.Container`) vs `handlers.Container`, constructed field-by-field in **two** places (`app/application.go:55` and `:129`). Adding a service means touching three sites; they will drift. Build the handlers container once from the app container.

### M-T9 — Inconsistent DI and layering
`handlers.Container` mixes interfaces (`dhcp.ServerService`) with concrete types (`*ipxe.Service`, `*config.Config`) — the concrete deps hurt testability; `DHCPHandlers` destructures the container into fields while every other handler keeps `*Container`. `syslinux`'s impl struct is unexported while `osimage`'s is exported; `tftp.NewServer` returns a concrete `*Server` while the others return interfaces. Pick one pattern per layer.

### M-T10 — Duplicated logic across handlers
IP-sort comparator copy-pasted in `sortServerViewsByIP` and `convertLeasesToViews` (`handlers/dhcp.go:520-594`); server→view conversion duplicated in `HandleDHCPPage`/`GetDHCPServers` (`:32-98`); "find server by network IP" reimplemented in `common.go:NewReserveModal`, `ipmi.go:updateDHCPLeaseWithIPMI`, `bootmenu.go:updateDHCPLease`.

### M-T11 — Inconsistent error handling
Half the handlers use `AppError`/`HandleError`; the rest use raw `http.Error(w, err.Error(), …)` leaking internals (`dhcp.go:StartDHCPServer`, `osimage.go:OSImagesPage`). `ErrorTypeRateLimit` is defined but rate limiting exists nowhere; `Login` has no body-size limit.

### M-T12 — Templates re-parsed per request; `template.Must` panics the server
`handlers/common.go:32-52` (`LoadTemplates`) re-parses ~18 files on every page load (perf), and `template.Must` means one missing `.templ` file panics on any page load instead of at startup. Parse once at startup.

### M-T13 — Every lease lookup is a full table scan — per DHCP packet
`dhcp/repository.go`: `GetByMAC`/`GetByIP`/`GetByServerID`/`GetExpired` all `GetAll` (full bucket read + JSON unmarshal of every lease) then filter in memory; `isIPAvailable` calls `GetByServerID` per candidate IP in a loop. Fine for 10 leases, a latency cliff for hundreds. Add a MAC secondary index.

### M-T14 — `HandleDownload`'s validator rejects legitimate downloads
`handlers/tftp.go:64-72` + `handlers/security.go:74-99`: `ValidateTFTPPath` runs `ValidatePathWithinBase` on the *relative* input — `filepath.Abs` resolves against CWD, not `TFTPDir` — so normal filenames fail containment and the correct check (`GetSafePath`, which joins first) is never reached.

### M-T15 — Misc type smells
`config.HTTPConfig.Port` is an unvalidated `string` (`HTTP_PORT=bogus` fails only at listen); "immutable" `config.Build()` shares the `OSImages.Sources` map; the OS catalog is hundreds of hardcoded lines that belong in a data file; `getServerIP` picks the first non-loopback NIC (may be a docker bridge) with no override; `ipxe` `getKernelArgs` hardcodes `:8080`; `HealthCheck.LastCheck` (`interfaces/service.go:33`) is a non-pointer `time.Time` (zero value ≡ "never checked" in JSON); `dhcp` `GetStateBadgeClass` returns CSS classes from the domain model (presentation leaking into `dhcp`); `SetActiveVersion` (`syslinux/repository.go`) ignores `json.Marshal` errors.

---

## Minor

- `osimage`: dead `activeDownloads` map; `downloadFile(url, filepath string)` shadows the `path/filepath` import; hardcoded `Architecture: "x86_64"` ignoring `osConfig.Architecture`; lexicographic `sort.Strings` version sort (`"10"` < `"9"`); queue-full path ignores `Save` error; `GetByOS`/`GetByOSAndVersion` full scans; `SetDefault` non-atomic (two actives possible).
- `syslinux`: `GetBootTypeFromVersion` does lexicographic `version < "4.00"` (`"10.0" < "4.00"` is true); `ParseVersionFromFilename` assumes the `.tar.gz` suffix (`len-7`); `RemoveBootFiles` hardcodes `"boot-bios"`/`"boot-efi"` instead of `s.config` dirs; `extractSingleFile` uses `context.Background()` not the passed ctx; download IDs `fmt.Sprintf("download-%s-%d", version, time.Now().Unix())` collide within a second; repositories mutate caller structs (`UpdatedAt` inside tx).
- `tftp`: double `Start()` overwrites `s.listener` (socket leak); `Stop()` doesn't wait for the serve goroutine; the 100ms "started OK" probe races slow bind failures.
- `dhcp`: `UpdateServer` dead assignments (`config.IP = server.IP` then `server.IP = config.IP`); restart-after-update failure only logged, server left stopped; `validateServerConfig` never checks valid IPv4/mask/StartIP-in-subnet; `Stop()` before `Start()` blocks 5s on nil ctx; serve-loop errors only logged; `ReserveLease` ignores `DeleteByMAC` error; `lease == nil` checks after `GetByMAC` are dead code; `incrementIP` truncates IPv6/panics on odd lengths; `MarkOfflineLeases` ignores `Save` errors; `IsActive` treats `StateComplete` as active so `MarkOfflineLeases` flips provisioned machines to `offline`; `UpdateState` calls `time.Now()` 3× per transition; `LeaseRange` unchecked — `startInt + uint32(s.LeaseRange)` can wrap past `255.255.255.255`; `GetLeasesByState` scans per server and `continue`s past errors (partial results as success).
- `db`: `bolt.Open(path, 0744, …)` — world-readable DB; should be `0600`. `ctx` params are decorative (bbolt has no ctx API). `GetAllKV` returns empty map on missing bucket while siblings error — inconsistent.
- `handlers`: `removeLastDir` (`tftp.go:40`) runs after the TFTP prefix was trimmed, so `PrevDirectory` is always `""` (broken breadcrumbs); `ipToInt` (`common.go`) silently truncates IPv6 to 4 bytes; `sortServerViewsByIP` would panic on nil `net.ParseIP`; `ValidateFileType` (`security.go:175`) allowlists by substring (`strings.Contains(base, "vmlinuz")` matches `evil-vmlinuz.sh`); `ProvisionFileInfo.Path` exposes absolute server paths to the UI; `IsHTMLRequest` (`errors.go:176`) treats `Accept: */*` as HTML (curl gets text, not JSON errors).
- `app`: `log.Fatalf` inside the HTTP goroutine (`application.go:78`) → `os.Exit` with no cleanup; `Stop()` never stops DHCP protocol handlers (UDP listeners leak); `NewContainer` leaks the opened BoltDB if `syslinux.NewBoltRepository` fails.
- `config`: `validate()` only checks DB fields.

---

## Test quality

The suites are large but mostly test mocks, not production code:

- `interfaces/*_test.go` (~1200 lines) tests a hand-written `MockService` defined in the test files — canned state transitions of the mock itself. Tautological.
- `osimage/osimage_test.go`: `createTestService` deliberately skips starting the background worker, so `processDownload`, `downloadFile`, checksum handling, and the M-C1 race are never exercised. `TestCancelDownload` asserts only the mock-repo call path.
- `syslinux/syslinux_test.go`: model-struct tests + `TestMockRepository_*` (testing the mock again); `DownloadVersion`, `ExtractBootFiles`, `ScanMirror`, `InstallBootFiles` untested.
- `tftp/tftp_test.go`: `TestTFTPFileOperations` tests `os.WriteFile`/`os.ReadFile` (the stdlib); no RRQ/WRQ round-trip; `TestServerStart` skipped unless root. The CR-7 traversal bug has zero coverage.
- `ipxe/ipxe_test.go` is the strongest (template rendering, `WriteConfigToFile`) but misses the duplicate-ID case and empty-OS panic.
- `vmtest/vmtest_test.go` asserts operations *fail* (`TestCreateDHCPServerInIgnite`, `TestVerifyIgniteStatus` expect errors with no server) plus trivia (`TestColorConstants`, `TestShowHelp`).
- `main_test.go`: asserts `NotNil(staticFS)` on a struct value and `Nil(app)` on a nil pointer; `TestDataOperationsHandling` never calls `HandleDataOperations`.
- Net: the download state machines, TFTP handlers, and DHCP packet paths — the parts that actually break — are essentially untested. Fix the races first (untestable-by-mock races need integration tests: real bbolt, concurrent DISCOVERs via the packet path), then backfill.

---

## Suggested fix order

1. **CR-1** auth bypass + **CR-2…CR-7** file traversal/write — these are remotely exploitable today; do them as one security pass (server-side sessions + a single `safeJoin(base, name)` helper applied at every file endpoint, and actually wire up the existing `security.go` validators).
2. **CR-8, CR-9, CR-10** DHCP core — mutex the handlers map, set lease IDs on the packet path, serialize allocation (per-server mutex or bbolt transaction).
3. **M-C1, M-C2, M-C3** download state machines — context-based cancellation, download-then-swap, remove the no-op stubs or implement them.
4. **M-C8, M-C10, M-C11/M-C12** bbolt/API correctness.
5. **M-T1–M-T3, M-T6, M-T8** structural cleanup — delete dead interfaces, unify `DownloadStatus`, rename `testdata` → `cli`, one DI container.
6. Backfill tests around the fixed races with real concurrency (not mocks).

*This review is findings-only — no production code was modified. Line numbers refer to commit `3b4c996`.*
