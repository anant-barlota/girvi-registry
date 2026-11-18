# Girvi plugin catalog

The list of plugins for [Girvi](https://girvi.app/), for its Plugins screen to offer under **Browse**. It is one
file, [`index.json`](index.json), published at <https://plugins.girvi.app/index.json>.

The catalog holds no programs. Each version points at a plugin bundle that its author released on GitHub, with
the bundle's SHA-256 and what its `plugin.json` asks for, so Girvi can show that before downloading anything and
check the download before keeping it. Nothing runs until the user approves it.

The format, its rules and the tools are in [girvi-sdk](https://github.com/anant-barlota/girvi-sdk), package
[`plugin/registry`](https://github.com/anant-barlota/girvi-sdk/tree/main/plugin/registry).

## List your plugin

1. **Build it** with [girvi-sdk](https://github.com/anant-barlota/girvi-sdk). Its `plugin.json` needs a
   `description` and an `author`; the catalog shows both.
2. **Release it** with the SDK's release workflow: push a tag that matches `version` in `plugin.json`.
3. **List it.** Fork this repository and run, from the plugin's folder:

   ```sh
   girvi plugin publish --repo your-github-name/your-repo --index path/to/girvi-registry/index.json --license MIT
   ```

   It downloads the released bundle, checks it is that version, and adds it to `index.json`. Open a pull
   request. A check downloads the bundle again and compares it, so a mistake shows up before review.
4. **A maintainer reviews it.** The review is the only gate, so it is a real one: what the plugin asks for, whether
   its source is public, and whether it does what it says.

## Rules

- **The id belongs to your GitHub account.** `your-github-name/anything`, and the bundle must be a release asset of
  that account. `girvi/...` is reserved for official plugins.
- **A published version never changes.** Girvi users may have installed it. To fix a mistake, release a new version.
  A version can be removed if it is harmful.
- **The catalog repeats the bundle.** Its `api`, `events`, `fields` and `http_hosts` must equal the bundle's
  `plugin.json`; `girvi plugin publish` copies them and the check compares them.

`girvi registry check` runs the same checks locally (`--offline` skips the downloads).

## License

MIT. See [LICENSE](LICENSE).
