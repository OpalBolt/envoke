# Changelog

## [0.8.0](https://github.com/OpalBolt/envoke/compare/v0.7.0...v0.8.0) (2026-05-24)


### Features

* **providers:** add config:// URI scheme for YAML config template rendering ([#20](https://github.com/OpalBolt/envoke/issues/20)) ([#69](https://github.com/OpalBolt/envoke/issues/69)) ([6c140a3](https://github.com/OpalBolt/envoke/commit/6c140a3a8f2201c4904daaf71835a4443cee75d4))

## [0.7.0](https://github.com/OpalBolt/envoke/compare/v0.6.0...v0.7.0) (2026-04-29)


### Features

* **config:** migrate to envoke path/prefix, add config subcommand ([#61](https://github.com/OpalBolt/envoke/issues/61)) ([#62](https://github.com/OpalBolt/envoke/issues/62)) ([12fa4f8](https://github.com/OpalBolt/envoke/commit/12fa4f8f0ea18aea9b4af5d18d94dd7703f28df1))

## [0.6.0](https://github.com/OpalBolt/envoke/compare/v0.5.0...v0.6.0) (2026-04-28)


### Features

* **cli:** unify renv and kctx into single envoke command surface ([5a9ed8e](https://github.com/OpalBolt/envoke/commit/5a9ed8e79e48bd0b16e855189ff117ae4e1a793e)), closes [#54](https://github.com/OpalBolt/envoke/issues/54)
* **cli:** unify renv and kctx into single envoke command surface ([#54](https://github.com/OpalBolt/envoke/issues/54)) ([42c10e4](https://github.com/OpalBolt/envoke/commit/42c10e49a39ae2f97a29b76e7ee2a2e6e1701356))

## [0.5.0](https://github.com/OpalBolt/envoke/compare/v0.4.2...v0.5.0) (2026-04-28)


### Features

* add MIT license and README license section ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))
* **shell-init:** add terminal check to prevent unsafe direct eval ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))
* **ui:** add Braille spinner for long-running Bitwarden operations (TTY-gated, thread-safe) ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))
* **ui:** add BW spinner, dynamic terminal width, and MIT license ([#41](https://github.com/OpalBolt/envoke/issues/41), [#36](https://github.com/OpalBolt/envoke/issues/36)) ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))
* **ui:** dynamic terminal width detection; box width clamped to terminal width ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))


### Bug Fixes

* **resolve:** abort on terminal stdout instead of warning; add --force escape hatch ([#36](https://github.com/OpalBolt/envoke/issues/36)) ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))
* **ui:** guard strings.Repeat against negative repeat counts on narrow terminals ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))
* **ui:** truncate keys longer than 24 chars before rendering ([#41](https://github.com/OpalBolt/envoke/issues/41)) ([76fa842](https://github.com/OpalBolt/envoke/commit/76fa84289a054be3e493faade884bf23d7adb93a))

## [0.4.2](https://github.com/OpalBolt/envoke/compare/v0.4.1...v0.4.2) (2026-04-26)


### Performance Improvements

* **bitwarden:** cache folder/collection item lists in BWClient ([#51](https://github.com/OpalBolt/envoke/issues/51)) ([ed9f13e](https://github.com/OpalBolt/envoke/commit/ed9f13ed49d275eedcf8bd9524a93c3b97066051))
* **bitwarden:** cache folder/collection item lists in BWClient [#51](https://github.com/OpalBolt/envoke/issues/51) ([a7d7aa1](https://github.com/OpalBolt/envoke/commit/a7d7aa112becbf2ea752ae1c01e4f2e616798dbc))

## [0.4.1](https://github.com/OpalBolt/envoke/compare/v0.4.0...v0.4.1) (2026-04-23)


### Bug Fixes

* **flake:** update vendorHash for current dependency tree [#47](https://github.com/OpalBolt/envoke/issues/47) ([#48](https://github.com/OpalBolt/envoke/issues/48)) ([1776586](https://github.com/OpalBolt/envoke/commit/1776586ad1c143866696a5d40f85fe2f823e5944))

## [0.4.0](https://github.com/OpalBolt/envoke/compare/v0.3.0...v0.4.0) (2026-04-23)


### Features

* **securedir:** platform-aware secure storage abstraction ([#34](https://github.com/OpalBolt/envoke/issues/34)) ([#44](https://github.com/OpalBolt/envoke/issues/44)) ([1b6141c](https://github.com/OpalBolt/envoke/commit/1b6141c5d6a23536190c7ea5db6f41c711b7e791))

## [0.3.0](https://github.com/eficode/envoke/compare/v0.2.0...v0.3.0) (2026-04-10)


### Features

* add kctx.sh snippet for ephemeral Vault-backed kubeconfig switching ([deb2d9a](https://github.com/eficode/envoke/commit/deb2d9a4f409a44f1a7d625605a28c3f7b1f6f1a))
* Add nix flake ([b760f88](https://github.com/eficode/envoke/commit/b760f88ac4c0f50527865ece146a2d3feff30207))
* add Python drop-in and bash resolve_yaml_value for script usage ([b210061](https://github.com/eficode/envoke/commit/b2100611be981abee65bb26ad387a7f1673ecdba))
* add shell auto-detection and env tracking to resolve-env-refs.sh ([72a3bf2](https://github.com/eficode/envoke/commit/72a3bf2de8643799cfa26908a7aaa38ff4e8b7a8))
* **ai:** Add copilot-instructions ([ad9a7f3](https://github.com/eficode/envoke/commit/ad9a7f3dfe93b9c05850d17ac4b8c76dc028d096))
* Bitwarden SDK examples, .env reference pattern, resolve-env-refs.sh ([3ab5ee7](https://github.com/eficode/envoke/commit/3ab5ee7812c7c29684eacef227a81c84a6200b9d))
* **cleanup:** wire up sleep/lock hooks via renv/kctx watch subcommand ([7584079](https://github.com/eficode/envoke/commit/75840793219a85f0874686798ad21fb4834fe71a)), closes [#1](https://github.com/eficode/envoke/issues/1)
* **cleanup:** wire up sleep/lock hooks via renv/kctx watch subcommand ([#8](https://github.com/eficode/envoke/issues/8)) ([41c10e8](https://github.com/eficode/envoke/commit/41c10e8a0a23e5c0d45bd538dc4f8cda086c5d50))
* envoke unified binary v0.2.0 ([#18](https://github.com/eficode/envoke/issues/18)) ([01b8789](https://github.com/eficode/envoke/commit/01b878953464d453c6bc37ca2d25b89895d9f5f1))
* implement secure secrets handling resource repository ([9cc9c0f](https://github.com/eficode/envoke/commit/9cc9c0f7fa8c61e82fe5720408d6fc465220ed2e))
* improve CLI feedback with colors and richer status ([52823da](https://github.com/eficode/envoke/commit/52823daa2c60a329880776cb1757d8fb8f7969c2))
* **kctx:** add install script, README, caching, and Bitwarden support ([ecbd8f7](https://github.com/eficode/envoke/commit/ecbd8f7005e9585ec335fb4470707fbbe3ef1e00))
* **kctx:** named kubeconfig loading with kctx load / kctx switch ([#13](https://github.com/eficode/envoke/issues/13)) ([7f95bb3](https://github.com/eficode/envoke/commit/7f95bb30f891d03b062456378b897f642debcc63))
* **kctx:** use AES-256-CBC encrypted cache (same as resolve-env-refs) ([6c4fa38](https://github.com/eficode/envoke/commit/6c4fa385e1a1b5f6c69c217c6998e0b94747e742))
* rich colored output with lipgloss + docs revamp ([#14](https://github.com/eficode/envoke/issues/14)) ([55097a7](https://github.com/eficode/envoke/commit/55097a78ba18805cec2e9c94975c6cfb26b0a368))
* simplify to single resolve-env-refs.sh pattern ([f3ba425](https://github.com/eficode/envoke/commit/f3ba42569b7e6fcbb821c426bc4ebb3f8cba370c))


### Bug Fixes

* address registry lifecycle, Close deduplication, and misleading comments ([#24](https://github.com/eficode/envoke/issues/24)) ([2d9723a](https://github.com/eficode/envoke/commit/2d9723a4dd2cbbe7a0b1ec34ab0db010a355cab2))
* correct two regressions in _bw_ensure_session ([504d664](https://github.com/eficode/envoke/commit/504d664824cda17fff6db66170b3be210658c239))
* handle unauthenticated BW state in _bw_ensure_session ([0586eda](https://github.com/eficode/envoke/commit/0586eda5a9bca6cae13766ee9fa302e0fc58216f))
* **kctx:** shell function routing and stale-sentinel unload race ([107b4bd](https://github.com/eficode/envoke/commit/107b4bd7ee172df10056558bb36bfc3d51e0a56c))
* **kctx:** show root help for no-args; guard EXIT trap on switch failure ([338063f](https://github.com/eficode/envoke/commit/338063f7f7309d61273d3736c969aa40883e1c6f))
* **kctx:** strip trap from eval; move EXIT cleanup to top-level trap ([#16](https://github.com/eficode/envoke/issues/16)) ([39b992d](https://github.com/eficode/envoke/commit/39b992dc4ab8e41bf28e493cfc0c23b22e03256f))
* surface bw unlock errors and fix silent secret-resolution failures ([7062be8](https://github.com/eficode/envoke/commit/7062be8778154d0fdda3d6498b7d981d0050e0ba))
* use bw list items to avoid org-key null-pointer crash ([db95617](https://github.com/eficode/envoke/commit/db95617da42a3fea1306ae25c1d813f94c5bc135))
