# Provenance

`list.actual.html` and `robots.txt` are unmodified responses from the shared Fetcher inspection session. See requests.json for actual timestamps, URLs and counts. No detail page or attachment was requested.

The list exposes no separate collection/reuse policy link in its menu/footer, only “Copyright Soongsil University All Rights Reserved.” Robots allows notices but is not a reuse authorization. The user's explicit unclear-policy stop condition applies. Live crawling is blocked pending policy review.

`detail.synthetic.html` is a synthetic test document, NOT actual SSU detail HTML. `.fixture-notice-body` tests the extraction engine only and MUST NOT be used as a production SSU selector. A verified `detail_body_selector` and `policy_approved=true` are required before enabling the real source. The real detail selector and attachment layout remain unverified.

## User-supplied detail fixture (pending)

Place `detail.actual.html` here. After inspecting the actual DOM, copy the blank
`detail.actual.expected.json.example` to `detail.actual.expected.json` and fill
reviewed selectors, exact expectations and capture provenance. Do not copy the
synthetic selector. See `docs/detail-fixture.md` at the project root for browser
save instructions and offline tests. Both actual files are currently absent;
actual-detail tests skip explicitly. A partial submission fails.

## Actual detail supplied (supersedes the pending status above)

The user supplied `detail.actual.html` as pasted HTML. It is preserved byte for
byte. The matching expectation JSON records SHA256 and reviewed selectors.
The original URL is embedded in `input#copyurl`; capture time is unknown and
explicitly recorded as such. No network requests were used to obtain or inspect
this file. The department is absent from detail HTML and comes from the same
slug in `list.actual.html`, with that file's SHA256 also checked. The linked
images contain no alt text; no OCR was performed. See `docs/detail-fixture.md`
for the observed selectors, extraction limits and current validation commands.
