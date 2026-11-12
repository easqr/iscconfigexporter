# ISC Configuration Exporter

Export SailPoint Identity Security Cloud (ISC) configuration to local files using
the official SailPoint Go SDK. Each export saves the configuration objects and a
`manifest.json` containing their types, counts, and checksums. You can run the
command-line tool or use the Go module in your own application.

Start with [your first export](#build-and-run) below. To integrate the module into
an application, go to [Go library usage](#library-usage).

## Before you start

You will need:

- Go **1.27.4 or newer** and Git to build from source.
- Network access to download dependencies and connect to your ISC tenant.
- Your tenant's API URL and SDK client credentials with permission to export
  configuration (`sp:config:read`). Confirm the required tenant role with your administrator.
- Private local storage for the exported configuration. The exporter is tested
  on macOS; Windows and network filesystems are unsupported. See
  [storage requirements](docs/OPERATIONS.md#snapshot-storage).

Exports can contain secrets. Choose storage that only authorized users can
access. Exported values are saved without redaction or encryption.

## Build and run

### 1. Get the source and build

Clone the v0.8.0 release and build the command-line tool:

```sh
git clone --branch v0.8.0 https://github.com/easqr/iscconfigexporter.git
cd iscconfigexporter
go build -o isc-exporter ./cmd/isc-exporter
```

If you already have a checkout, run the build command from its root directory.
Run the remaining commands from that same directory so the tool can find your
configuration files.

### 2. Choose what to export

Create an exporter settings file:

```sh
cp exporter.example.json exporter.json
```

Edit `exporter.json`. For a first export, you can use these settings:

```json
{
  "tenant": "example",
  "output": "exports",
  "include": ["ROLE", "SOURCE"]
}
```

Replace `example` with your tenant's identity, matching the tenant returned in
exports. This example exports roles and sources; both types must be supported
by your tenant. To export all available types, use `"include": []`.
Backup evaluation is disabled when omitted. Other settings use their defaults;
see the [configuration reference](docs/CONFIGURATION.md#exporter-policy).

`output` is relative to the directory where you run the command. With the
settings above, the current snapshot will be available at `exports/example`.

### 3. Configure the connection

Create a separate file named exactly `config.json` in the same directory:

```json
{
  "ClientId": "<client-id>",
  "ClientSecret": "<client-secret>",
  "BaseURL": "https://<tenant>.api.identitynow.com"
}
```

Replace all three placeholders with your SDK connection settings. Confirm that
the API URL and the tenant in `exporter.json` refer to the tenant you intend to
export. Keep credentials in `config.json`, separate from `exporter.json`.
Protect the credentials file:

```sh
chmod 600 config.json
```

The repository ignores `config.json` in Git. If `SAIL_BASE_URL` is set, the tool
uses the `SAIL_*` environment settings as a whole instead of this file. See
[connection lookup and environment variables](docs/CONFIGURATION.md#native-sdk-connection-and-authentication).

### 4. Validate settings and run

Check the local settings first:

```sh
./isc-exporter -config exporter.json -validate
```

Expect `configuration valid`. This checks settings and options without loading
credentials or contacting ISC; it does not confirm authentication or permissions.

Run the export:

```sh
./isc-exporter -config exporter.json
```

The tool starts an ISC export job, waits for it to complete, and saves the
snapshot. Success produces a JSON result in the terminal and an exit status of
`0`. Look for `exports/example/manifest.json`, replacing `example` with your
tenant. If the export fails, start with [troubleshooting](docs/OPERATIONS.md#troubleshooting).

Use `./isc-exporter -help` for options and `./isc-exporter -version` for build
information. Neither requires credentials. Ctrl+C requests cancellation; it does
not cancel the remote ISC job. SDK token acquisition has separate
[timeout and cancellation limits](docs/CONFIGURATION.md#authentication-limits).

## Output

With the example settings, a successful export produces:

```text
exports/
  example -> .isc-snapshots/example-<generation>
  .isc-snapshots/example-<generation>/
    ROLE/id-<hex-of-self.id>.json
    SOURCE/id-<hex-of-self.id>.json
    manifest.json
```

Each object file contains the configuration and its original metadata. Filenames
encode the object's ID by default; the ID and name are readable inside the JSON. The
manifest lists selected types, object counts, file checksums, and paths created,
modified, unchanged, or deleted since the previous export. Types with no objects
have no object files.

`exports/example` is a symbolic link to the latest saved snapshot. Rerunning the
command saves a new version and updates that link after the files are ready.
Older versions remain on disk. Changing type filters changes which objects
appear in the current snapshot.

Keep saved snapshots read-only. Unchanged files can share storage with older
versions, so editing one can alter several saved exports. Make a separate copy
before editing. See [snapshot files and readers](docs/SNAPSHOTS.md) for file
formats, and [storage and recovery](docs/OPERATIONS.md#snapshot-storage) for
managing saved versions.

## Filename options

Choose `"filename_mode": "hex"` (default), `"id"`, or `"name"` in the exporter
policy, or override it for one run:

```sh
isc-exporter -config exporter.json -filename-mode id
isc-exporter -config exporter.json -filename-mode name
isc-exporter -config exporter.json -check-filenames
```

ID mode writes `<TYPE>/<self.id>.json`; name mode writes
`<TYPE>/<self.name>.json` by default. When names would collide, every member of
that collision group receives a `--<self.id>` suffix. Set `"filename_append_id": true`
in `exporter.json` to force the suffix on every name-mode file (default `false`).
Lifecycle-state filenames also prepend the identity profile name, for example
`LIFECYCLE_STATE/Employees--Active.json`. Both are checked against
Linux, macOS and Windows filename rules and conservative case/Unicode collision
checks. Any issue fails the export before publication, with affected objects listed;
values are never silently sanitized or replaced with a fallback.
Set `"filename_escape": true` in `exporter.json` to reversibly escape unsafe
characters (for example, `:` becomes `%3A` and literal `%` becomes `%25`).
The default is `false`; hex filenames are unaffected. Escaped filenames still
undergo length and collision checks.

`-check-filenames` assesses both ID and name modes and prints a JSON report. It
starts and downloads an ISC export job, but publishes no local files and performs
no backup evaluation. Exit status is 1 if either mode has issues, 0 if both pass.
`-validate` remains an offline policy check, including when combined with these
options. See [filename compatibility](docs/SNAPSHOTS.md#filename-compatibility)
for report fields, limitations, and reader compatibility.

## Library usage

Add the published module from your application's Go module directory:

```sh
go get github.com/easqr/iscconfigexporter@v0.8.0
```

Follow the [Go library guide](docs/LIBRARY.md) to configure the SDK, run an export,
handle errors, and read snapshots. Reading existing snapshots uses the
SDK-free `github.com/easqr/iscconfigexporter/snapshot` package and needs no tenant
credentials.

## Optional workflows

- [Retrieve Configuration Hub mappings](docs/MAPPINGS.md): save mappings in a
  separate archive. This requires additional read permission; ordinary exports
  do not request mappings.
- [Evaluate Configuration Hub backups](docs/CONFIGURATION.md#backup-evaluation):
  list and evaluate backups after an export. The supplied integration is
  read-only and cannot delete backups. `-cleanup-preview` still runs an export
  and saves a snapshot.

The exporter saves configuration locally. Importing or restoring it, comparing
environments, scheduling exports, and publishing to Git or remote storage require
separate tools or workflows.

## Further documentation

- [Configuration reference](docs/CONFIGURATION.md): filters, timeouts, credentials, and backup settings.
- [Operations and troubleshooting](docs/OPERATIONS.md): results, failures, storage, and recovery.
- [Documentation index](docs/README.md): library, format, API, and contributor references.

Files are saved with permissions `0600` and directories with `0700`. Logs omit
object payloads and raw API error bodies; SDK credentials are not written to
manifests. Exported content is not encrypted or signature-verified. Use private
storage and an HTTPS API URL you trust.

## License

Source code and documentation are licensed under the
[Apache License, Version 2.0](LICENSE). Third-party dependencies retain their own
licenses; see [dependency licenses and notices](docs/DEPENDENCIES.md).
