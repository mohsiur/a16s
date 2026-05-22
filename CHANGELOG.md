# Changelog

## [0.1.2](https://github.com/mohsiur/a16s/compare/v0.1.1...v0.1.2) (2026-05-22)


### Features

* **view:** add S3 buckets kind ([#43](https://github.com/mohsiur/a16s/issues/43)) ([c7410d2](https://github.com/mohsiur/a16s/commit/c7410d2fd124ce74f237df121592d3d3f28d795a))


### Bug Fixes

* don't crash `a16s --version` / `--help` when GitHub is unreachable ([bd5d474](https://github.com/mohsiur/a16s/commit/bd5d474e43e0ccbbd6bd5006dde6f0c53a7a8e6a))
* DynamoDB index Enter now drills into items again ([38fa939](https://github.com/mohsiur/a16s/commit/38fa939b63042514f79908bf090c3edf7f8a0598))
* **version:** degrade gracefully when GitHub is unreachable ([#21](https://github.com/mohsiur/a16s/issues/21)) ([bd5d474](https://github.com/mohsiur/a16s/commit/bd5d474e43e0ccbbd6bd5006dde6f0c53a7a8e6a))
* **view:** cancel auto-refresh ticker on shutdown ([#24](https://github.com/mohsiur/a16s/issues/24)) ([6492680](https://github.com/mohsiur/a16s/commit/6492680f65d8ec4ad4adb76af9cbf03953a2ed80))
* **view:** restore DynamoDB index drill to items ([#44](https://github.com/mohsiur/a16s/issues/44)) ([38fa939](https://github.com/mohsiur/a16s/commit/38fa939b63042514f79908bf090c3edf7f8a0598))

## [0.1.1](https://github.com/mohsiur/a16s/compare/v0.1.0...v0.1.1) (2026-05-21)


### Bug Fixes

* arrow keys now scroll columns horizontally on the DDB scan, SQS ([d6439cb](https://github.com/mohsiur/a16s/commit/d6439cb7d2884b0e5ae78d8164ffd70722ef1fc1))
* **view:** arrow keys scroll columns on leaf flat-kind tables ([#15](https://github.com/mohsiur/a16s/issues/15)) ([d6439cb](https://github.com/mohsiur/a16s/commit/d6439cb7d2884b0e5ae78d8164ffd70722ef1fc1))
