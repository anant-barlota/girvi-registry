# Girvi plugin catalog

The list of plugins the [Girvi](https://girvi.app/) app shows under **Plugins, Browse**.
It is one file, [`index.json`](index.json), published at <https://plugins.girvi.app/index.json>.

The catalog holds no programs. Each plugin's author builds and hosts their own, as a GitHub Release. The catalog
points at them and records a SHA-256 for each, which Girvi checks before it keeps a download. Nothing downloaded
runs until the user approves it.

## List your plugin

1. **Build a plugin** with the [Girvi plugin SDK](https://github.com/anant-barlota/girvi-plugins). The
   [webhook plugin](https://github.com/anant-barlota/girvi-webhook) is a small complete example.
2. **Release it.** Copy [its release workflow](https://github.com/anant-barlota/girvi-webhook/blob/main/.github/workflows/release.yml)
   into your repository and push a tag such as `v0.1.0`. It builds for Windows, macOS and Linux and attaches the
   programs to a GitHub Release. The tag must equal the version your plugin reports.
3. **Add it here.** Fork this repository and run:

   ```bash
   go run ./tool add \
     -id your-github-name/your-plugin -version 0.1.0 \
     -repo your-github-name/your-repo -asset girvi-plugin-yourname \
     -name "Your Plugin" -author "You" -description "One line about what it does." \
     -license MIT -homepage https://github.com/your-github-name/your-repo \
     -capabilities events.loan.created -requires network
   ```

   This downloads each program and fills in its checksum and size. Open a pull request. A check downloads every
   program again and confirms it matches, so a mistake shows up before review.
4. **A maintainer reviews it.** The review is the only gate, so it is a real one: what the plugin asks for, whether
   its source is public, and whether it does what it says.

To publish a new version, release it the same way and run `tool add` with the new `-version`.

## Rules

- **The id belongs to your GitHub account.** `your-github-name/anything`, and the downloads must be release assets of
  that account. `girvi/...` is reserved for official plugins.
- **A published version never changes.** Girvi users may have installed it. To fix a mistake, release a new version.
  A version can be removed if it is harmful.
- **Declare what the plugin asks for** with `-capabilities` and `-requires`. Girvi shows this before install and then
  reads the plugin's own declaration when the user approves it, so the two must agree.
- **Built for the current contract.** `-protocol` is the plugin contract version the plugin was built for (3 today).
  Girvi only offers versions that match the contract it speaks.

## Format

```json
{
  "schema": 1,
  "name": "Girvi official",
  "plugins": [
    {
      "id": "owner/name",
      "name": "Shown to users",
      "author": "Shown to users",
      "description": "One line.",
      "license": "MIT",
      "homepage": "https://...",
      "versions": [
        {
          "version": "1.2.3",
          "protocol": 3,
          "capabilities": ["events.loan.created"],
          "requires": ["network"],
          "artifacts": [
            { "os": "windows", "arch": "amd64", "url": "https://github.com/...", "sha256": "…", "size": 123 }
          ]
        }
      ]
    }
  ]
}
```

Platforms are `linux`, `darwin` and `windows` on `amd64` or `arm64`. Run `go run ./tool check` to validate the file
before you open a pull request (`-offline` skips the downloads).

## Your own catalog

Anyone can run a catalog: host an `index.json` like this one at an https address and add it in Girvi under
**Browse, Add a catalog**. If you sign it, give users the public key so Girvi refuses a changed copy.

## License

MIT. See [LICENSE](LICENSE).
