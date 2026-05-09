## [0.5.3](https://github.com/Charles546/hd-driver-podman/compare/v0.5.2...v0.5.3) (2026-05-09)


### Bug Fixes

* extract sha256 from asset digest ([9b1b734](https://github.com/Charles546/hd-driver-podman/commit/9b1b73463872f03366af9f3d74a10b74b5afbf8b))

## [0.5.2](https://github.com/Charles546/hd-driver-podman/compare/v0.5.1...v0.5.2) (2026-05-09)


### Bug Fixes

* have to quote the inline template ([c207c46](https://github.com/Charles546/hd-driver-podman/commit/c207c4685b03f76d267f060e3cfef866147c7caf))
* use hdci limited template variables in hdci yml ([1e248c6](https://github.com/Charles546/hd-driver-podman/commit/1e248c685e58fae2d72d86968429b7a9303bbe8e))

## [0.5.1](https://github.com/Charles546/hd-driver-podman/compare/v0.5.0...v0.5.1) (2026-05-08)


### Bug Fixes

* avoid commits with skipci trigging release ([a509a8c](https://github.com/Charles546/hd-driver-podman/commit/a509a8cefca18136625f49a4001d553667b6844a))

# [0.5.0](https://github.com/Charles546/hd-driver-podman/compare/v0.4.3...v0.5.0) (2026-05-08)


### Features

* relay release event to honeydipper-registry ([a0574a7](https://github.com/Charles546/hd-driver-podman/commit/a0574a7dc220d11a09ee8d5a0d3af3e92298be52))

## [0.4.3](https://github.com/Charles546/hd-driver-podman/compare/v0.4.2...v0.4.3) (2026-05-06)


### Bug Fixes

* correct cp destination ([#13](https://github.com/Charles546/hd-driver-podman/issues/13)) ([512a35b](https://github.com/Charles546/hd-driver-podman/commit/512a35bee5f498c36d07fc512b051bca644307da))

## [0.4.2](https://github.com/Charles546/hd-driver-podman/compare/v0.4.1...v0.4.2) (2026-05-06)


### Bug Fixes

* semantic release retrieve binary from cache ([#12](https://github.com/Charles546/hd-driver-podman/issues/12)) ([f702863](https://github.com/Charles546/hd-driver-podman/commit/f702863d330d2674723306abed3f9d47365959a1))

## [0.4.1](https://github.com/Charles546/hd-driver-podman/compare/v0.4.0...v0.4.1) (2026-05-06)


### Bug Fixes

* add binary during release ([#11](https://github.com/Charles546/hd-driver-podman/issues/11)) ([7f1516d](https://github.com/Charles546/hd-driver-podman/commit/7f1516d87268bc741950ce8b54f06720fe00b345))

# [0.4.0](https://github.com/Charles546/hd-driver-podman/compare/v0.3.0...v0.4.0) (2026-05-05)


### Bug Fixes

* **ci:** correct manifest_file to hd-driver-podman.json ([60d690b](https://github.com/Charles546/hd-driver-podman/commit/60d690b966a48fb05aad3cebb3ef1171719aa9c9))
* **ci:** set driver_name=podman to match registry manifest filename ([d5e2c5d](https://github.com/Charles546/hd-driver-podman/commit/d5e2c5d6509446bfcb39385d9d189bb0da8482d8))
* **config:** required packages based on pkg manager ([#7](https://github.com/Charles546/hd-driver-podman/issues/7)) ([a814b3c](https://github.com/Charles546/hd-driver-podman/commit/a814b3cf6b3958453202b387aa9a53abaa214848))


### Features

* **config:** declare gpgme as a requiredPackage for remote acquire ([#6](https://github.com/Charles546/hd-driver-podman/issues/6)) ([b458dcc](https://github.com/Charles546/hd-driver-podman/commit/b458dcc5f8088a8f231d3dd5d86b921ec7810288)), closes [honeydipper/honeydipper#697](https://github.com/honeydipper/honeydipper/issues/697)
