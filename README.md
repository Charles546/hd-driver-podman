# Honeydipper Driver for Podman

## Configuration

The repository config in [config/init.yaml](/home/charles/code/hd-driver-podman/config/init.yaml) now uses a single entrypoint for both deployment modes:

- Default: download the driver from the named remote registry `charles-gh-pages`
- Optional: switch to a pre-fetched local binary by loading the repo with `options.use_local_binary: true`

Example repo entry:

```yaml
repos:
  - repo: https://github.com/Charles546/hd-driver-podman.git
    options:
      # Optional. Omit this line to use the registry-backed default.
      use_local_binary: true
```

Optional remote overrides can also be passed through repo options:

```yaml
repos:
  - repo: https://github.com/Charles546/hd-driver-podman.git
    options:
      registry: charles-gh-pages
      channel: stable
      # version: v0.1.0
```

## Commercial licensing

If your intended use does not fit AGPL obligations, see `LICENSE-COMMERCIAL.md` and contact the copyright holder for commercial terms.

## License

This project is prepared for dual licensing:

- `LICENSE` — GNU Affero General Public License v3.0
- `LICENSE-COMMERCIAL.md` — commercial licensing path for organizations that want to use the software outside the AGPL terms

The AGPL license applies by default unless you have a separate written commercial agreement with the copyright holder.

