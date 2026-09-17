# Vendored htmx

The UI embeds and serves htmx locally; it does not fetch JavaScript from a CDN.

- Version: **1.9.2**
- Local asset: `web/static/htmx.min.js`
- Upstream release: <https://github.com/bigskysoftware/htmx/tree/v1.9.2>
- Upstream asset: <https://raw.githubusercontent.com/bigskysoftware/htmx/v1.9.2/dist/htmx.min.js>
- SHA-256: `fd346e9c8639d4624893fc455f2407a09b418301736dd18ebbb07764637fb478`
- License for this release: **BSD-2-Clause**, retained in `web/static/htmx.LICENSE.txt` and embedded in the binary. It is served at `/static/htmx.LICENSE.txt`.
- Upstream license: <https://raw.githubusercontent.com/bigskysoftware/htmx/v1.9.2/LICENSE>

The local asset was compared byte-for-byte with the tagged upstream file on
2026-09-17. To repeat the check from the repository root:

```sh
curl --fail --location https://raw.githubusercontent.com/bigskysoftware/htmx/v1.9.2/dist/htmx.min.js --output /tmp/webhookhub-htmx-upstream.min.js
sha256sum /tmp/webhookhub-htmx-upstream.min.js web/static/htmx.min.js
cmp /tmp/webhookhub-htmx-upstream.min.js web/static/htmx.min.js
```

To update, select an explicit upstream release, review its release notes and
license, download the tagged asset and license, and update the local files,
version, source URLs and checksum here. Review the diff and rebuild the binary
and Docker image: both embed the files at build time.

Verify login/session expiration, boosted navigation, dashboard filters and
pagination, auto-refresh, Inspect, replay/delete, and forwarding-rule forms in
the built image. A migration to htmx 2.x is a separate UI compatibility change.
