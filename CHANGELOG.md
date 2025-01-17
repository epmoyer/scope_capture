# Changelog, scope_capture

All notable changes to this project will be documented in this file.

The format is based on [Keep a
Changelog](http://keepachangelog.com/en/1.0.0/) and this project adheres
to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## Unreleased
(None)

## v0.0.6 2025-01-16
### Added
- Look for a config file at `./config.json` and then `~/.config/capture/config.json`.
    - If found:
        - Adopt `hostname` (if declared).  Type: string
        - Adopt `port` (if declared).  Type: int
- `-port` command line option.
### Changed
- Timestamp annotation color from white to light green-blue.
- (internal)
    - Refactor color declarations.
    - (build script)
        - Adopt `_` (instead of `.`) as separator for os and architecture. e.g. `scope_capture_v0.0.6_amd64_linux.bin`.

## v0.0.5 2025-01-15
### Changed
- Append `.exe` to the Windows build in `build.py`.
- Tighten up the spacing between annotation notes/labels.
- Increase character spacing by 1 pixel.

## v0.0.4 2025-01-15
### Changed
- Auto correct PNG checksum (an experimental fix to correct a RIGOL scope which appears to be generating bad PNG checksums).

## v0.0.3 2025-01-15
### Changed
- If a note is given on the command line, and an explicit filename is not given, use the note to name the output file.
- Distribution Build script:
    - Include version number in output binary name.
    - Build Linux AMD64 (not ARM64) as standard.
- If output filename exists, append `_{number}` (before the extension) to create a unique new output filename.
    - Will choose the next available `{number}` which does not yet exist on the filesystem.
## v0.0.2 2025-01-15
(Initial release)