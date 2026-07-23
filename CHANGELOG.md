# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.2] - 2026-07-23

### Changed

- Historical charts now open in the speed view while keeping the traffic view available as a manual toggle.

### Fixed

- Chart tooltips now show a readable timestamp and the appropriate speed or traffic unit.

## [0.2.1] - 2026-07-23

### Changed

- Refined the target-alias editor with a compact responsive layout and a single save action.
- Updated the browser favicon to match the FlowLens brand mark.

### Fixed

- Today and Yesterday history charts now use hour labels instead of repeating calendar dates.
- Clearing an existing target alias and saving now restores the original target name.

## [0.2.0] - 2026-07-23

### Added

- Optional trusted-LAN mode that disables shared-key login with `auth.enabled: false`.

### Changed

- Denser dashboard sizing, quieter chart styling, and more compact mobile cards.

### Fixed

- Realtime traffic updates no longer dispose and rebuild the chart every second.

## [0.1.0] - 2026-07-22

### Added

- Self-hosted dashboard for live speed, exact historical totals, approximate connection attribution, data quality, and SQLite storage state.
- Strict YAML configuration, same-origin shared-key sessions, health/readiness checks, and bounded Clash API collection.
- Multi-resolution SQLite rollups, retention, capacity protection, validated local backups, and manual restore tooling.
- Responsive embedded React interface with system, light, and dark themes.

### Security

- Non-root scratch container and hardened Compose defaults.
- Redaction boundaries for secrets, paths, session data, and public examples.

[Unreleased]: https://github.com/Willxup/flowlens/compare/v0.2.2...HEAD
[0.2.2]: https://github.com/Willxup/flowlens/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/Willxup/flowlens/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/Willxup/flowlens/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Willxup/flowlens/releases/tag/v0.1.0
