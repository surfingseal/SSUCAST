# Flash comparison experiment

`cmd/compare-schedule-flash` is a manual experiment using exactly the 76
identities frozen by the prior Flash-Lite experiment. It never registers a job
and never writes MongoDB or creates indexes. Existing 10 applied analyses must
pass projection/history validation; all 212 BSON documents must retain the same
snapshot hash. All 76 content/input hashes and original titles, bodies and
publication times must match before generation.

```
go run ./cmd/compare-schedule-flash --lite /absolute/lite-directory --existing /absolute/ten/apply-input.json --directory /absolute/new-flash-directory
# Explicit paid execution only, after preflight and review:
go run ./cmd/compare-schedule-flash --lite /absolute/lite-directory --existing /absolute/ten/apply-input.json --directory /absolute/new-flash-directory --execute
```

Model: `gemini-3.8-flash`, Vertex backend, global, existing ADC. No `.env`
changes, fallback, new crawling, attachment fetches or automatic execution.
The system instruction, schema, JSON notice input, output cap (16384) and SDK
version remain identical to Lite. Thinking is explicitly MEDIUM. Lite did not
specify thinking; therefore this is model-plus-thinking comparison rather than
an isolated model-only ablation. Both prompt and schema hashes are frozen.

The shared durable transport journal limits generation to 90 requests and 76
allowlisted notices, at most three transmissions per notice. Only 429/5xx are
retryable, with exponential delay. HTTP responses are stored before SDK decode;
a successful paid response can be recovered without another transmission. An
ambiguous transport/read/journal state blocks continuation. OS advisory locks
prevent simultaneous paid workers. No SDK retry/continuation is enabled.

Before every transmission, the guard reserves a conservative input allowance
(request UTF-8 bytes +32768) and twice the output cap for output/thoughts. Known
billed-token estimates plus this reservation must stay within US$3. Missing
usage on a successful response stops generation. Rates checked on 2026-11-06:
global introductory input $0.75/M, output including thinking $3.75/M, cached
input $0.075/M, through 2026-12-31. Estimates are not a billing guarantee/invoice.
Pricing must be reviewed before executing on another date.

Sources:
- https://cloud.google.com/model-garden/pricing
- https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/guides/gemini-3-8-flash
- https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/thinking
- Gemini official docs MCP: `gemini-api-guides/thinking.md#gemini-thinking/controlling-thinking-idthinking-levels`
- Installed `google.golang.org/genai@v1.77.0`: `ThinkingConfig`,
  `ThinkingLevelMedium`, `GenerateContentConfig.ResponseSchema`,
  `Models.GenerateContent`, response `UsageMetadata`.

Raw HTTP/AI output, validated v2 projections and source input are separate files.
Flash projections use response format `schedule-v2-flash-comparison-v1` and
cannot impersonate Lite provenance. Both models pass the same v2 validator for
comparison. The new offline quality policy remains an additional audit and is
not silently applied to only one model. Batch apply remains disabled even with
`--apply`; read-only CAS Preview is available. Summary/audience limitations and
unsupported numeric/date proposals must be reported separately from schema
success. No semantic accuracy/recall is asserted without independent gold data.
