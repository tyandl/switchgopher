# Working on SwitchGopher

Start with [`documentation/ARCHITECTURE.md`](documentation/ARCHITECTURE.md) for
how the pieces fit, then come back here for the day-to-day workflow.

## Setup

You need Go 1.27 or newer. SQLite is built in (pure Go, no C compiler needed).
Optional: Docker, and net-snmp (`sudo apt install snmpd` on Debian/Ubuntu) for
the simulator below.

```sh
git clone git@github.com:tyandl/switchgopher.git
cd switchgopher
go build ./... && go test ./...
```

If both pass, your environment is fine. Then follow the README quick start to
run the app, using the `*.example.json` files.

## The checks to run before you commit

```sh
gofmt -l .                      # must print nothing (gofmt -w . fixes it)
go vet ./...
go test ./...
```

GitHub Actions (`.github/workflows/ci.yml`) runs the first three, plus a build and
`go test -race`, on every pull request and every push to `main`, so a
formatting slip or a failing test shows up as a red check. It also builds the
Docker image, and runs `govulncheck` weekly. Run the same commands locally
first and CI will pass.

Also worth running now and then: `staticcheck ./...` (needs a release that
supports your Go version) and `govulncheck ./...` (known vulnerabilities in
dependencies and the standard library).

## Running against a fake switch

Real switches are not required for most work. net-snmp's `snmpd` can pretend to
be one. `testdata/snmpd.conf` is a minimal config for it.

```sh
# 1. Start the fake switch on a high port (no root needed).
snmpd -f -C -c testdata/snmpd.conf -Lf /tmp/snmpd.log udp:127.0.0.1:16161

# 2. In configuration/devices.json, add a device pointing at it:
#    {"Device":"test-sim","Category":"Core","Building":"Headquarters","Room":"R1",
#     "Model":"sim","Index":"01","Address":"127.0.0.1:16161","Community":"public"}
#    (the Building and Category must exist in buildings.json and categories.json)

# 3. Load it, then poll it once and look at the result.
go run ./cmd/switchgopher -seed
go run ./cmd/switchgopher -pollonce test-sim
```

`-pollonce` prints an error naming whatever failed, which is the fastest way
to debug a fetch change. Add `-log-level debug` to also see which optional
features the device does not support (that is normal for the simulator). What the simulator can and cannot do: it answers the
standard system and interface tables, but it has **no CDP and no forwarding
table (FDB)**, so those code paths are covered only by unit tests.

## Data for working on the web UI

The pages need ports, nodes and so on, which only a poll normally creates. For
UI work, seed a small site from the example files, then insert rows directly
with the `sqlite3` command line (the table layouts are at the top of
`internal/store/store.go`). Useful values: port `State` 1 = up, 2 = down;
`AdminState` 1 = enabled, 2 = disabled; `Duplex` 2 = half, 3 = full; `Since` and
node `TimeStamp` are Unix seconds. For a node to show up its `(Port, Device)`
must exist in `ports`. Run the app on a scratch copy (a different working
directory, or `-db /tmp/x.sqlite`) so you never touch real data.

## Common changes

**Add a page.**
1. Write a handler in `internal/web/handlers_*.go`. Follow an existing one:
   read query parameters, redirect home with `redirectHome` if a required one is
   missing, query with `?` placeholders, fill a `map[string]Any` that includes
   `Title` and `Version`, and execute the template.
2. Register the route in `router.go`.
3. Add `resources/templates/<name>.tmpl` using the shared `common_meta`,
   `common_heading_*` and `common_footer` blocks from `_base.tmpl`.
4. Link it from the nav bar in `_base.tmpl` (or from the page it belongs to).
5. If it needs a required parameter, add its handler to the table in
   `internal/web/redirects_test.go`.
6. Add a line about it to the Help page (`resources/templates/help.tmpl`).

**Add a value read from SNMP.** Decide which group it belongs to (or add a
group). Add the field to the type in `internal/model`, fill it in the generic
driver, add the column to the `CREATE TABLE` in `store.go` and to the matching
write-back function, then show it in a handler and template. If only one vendor
reports it, implement it in that vendor's driver instead; see
[`internal/snmp/drivers/README.md`](internal/snmp/drivers/README.md).

**Add a setting.** Add an entry to `configuration/preferences.json` with a
`description` ending in `; required: ..., type: ..., default: ...` and a
`value`, and read it with `config.Int` or `config.String`, passing the same
default.

**Change the database schema.** There are no migrations yet. Edit the
`CREATE TABLE` in `store.go`, and note that existing databases will not pick the
change up. Once real data exists, add a migration step to `Setup`.

## Conventions

- **Comments:** every exported identifier and every page handler has a doc
  comment that starts with its name. Comment *why*, not *what*; a comment that
  restates the code is noise. Package comments describe what the package is for.
- **SQL:** always use `?` placeholders. Never build a query by concatenating
  anything that came from a request or a config file.
- **Logging:** use `log/slog` (`slog.Info("seeded", "devices", n)`), never
  `fmt.Println` or the old `log` package, and put the details in key/value
  attributes rather than the message. Pick the level by the table in
  `internal/logging`: Debug for investigation detail, Info for normal events,
  Warn when something failed but the program carries on, Error when something
  failed that should not have. In a `main` function, `fatal(...)` logs at Error
  and exits. A fetcher whose device just lacks a feature wraps
  `snmp.ErrNotSupported` so it logs at Debug instead of Warn. **Never log a
  secret** (community string, passphrase, password, session cookie) or a
  request's query string or body. Logs written while handling a web request
  should include `"request_id", requestIDFrom(req.Context())`. Returning an error is better than logging it
  and also returning it: log where the error is finally handled.
- **Errors:** return them with context (`fmt.Errorf("doing X: %w", err)`);
  poller errors are logged and the next cycle retries. Handlers return
  `http.Error`.
- **No real data in the repo.** Examples and tests use fictional names (`HQ`,
  `LAB`, `HQ-R1-3560-01`) and the documentation address ranges (`192.0.2.0/24`,
  `198.51.100.0/24`, `203.0.113.0/24`). Real site config files, `.env` and the
  database are gitignored. Keep it that way.
- **Secrets** go in environment variables, referenced as `${NAME}` in
  `devices.json`. Never put a real community string or passphrase in a tracked
  file.
- **Tests:** add one for new behavior and for every bug you fix. Tests that
  touch the filesystem use `t.TempDir()` and `t.Chdir`.
- **Docs:** update the README if setup or usage changes. Record decisions and
  open work in `documentation/TODO.md`, in the same style as the existing
  sections.
- **Commits:** small and focused, with a message that says why.

## Docker

`docker compose build` builds the image from the `Dockerfile` (Go build stage,
small Alpine runtime, non-root user). See the README for running it.
