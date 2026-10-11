# Device configuration snapshots

Nothing in switchgopher pulls device configs itself yet - this directory is
just where it *looks* for them. Populate it with an external process (a
TFTP-received backup, an SSH-scraped `show running-config`, whatever you
already have) and the Config/Diffs pages will pick it up; leave it empty and
those pages render a clear "not available" message instead of failing.

Intent is for switchgopher to eventually absorb this itself (see
internal/web's configHandler/configsHandler/diffHandler/diffsHandler), at
which point this directory becomes its output rather than external input.

## Expected layout

- `<Category>/<Device>.txt` - the current/most recent config for a device
  (`configuration/preferences.json`'s `config_dir`, which points here).
- `dated/<Category>/<Device>.<NNN>-<NNN>.txt` - dated snapshots for history/
  diffing (`dated_config_dir`, points at `dated/`). The `<NNN>-<NNN>` suffix
  just needs to match `[0-9]*-[0-9]*` (see configsHandler) - it's not
  parsed as a real date, just used to list and sort snapshot filenames.

`Category` matches a device's `Category` column in the `devices` table
(e.g. `Core`, `Access` - see `configuration/categories.example.json`).
