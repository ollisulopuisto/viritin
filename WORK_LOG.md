# Work log

## 2026-11-17: preserve station ID mapping during allocation optimization

- Own adversarial review found the ASCII-only replacement changed non-ASCII
  and invalid UTF-8 IDs, unlike the original strings.ToUpper mapping.
- Added a conservative original-code fallback for non-ASCII input and permanent
  table regressions for normal UUIDs, short/empty/dash-only/malformed IDs.
- Tests fail before the fallback and pass after it. Full go test ./... and
  go vet ./... pass. BenchmarkRBUID: 48.84 ns/op,16 B/op,1 alloc/op locally.
- No device/receiver runtime tested. One hidden Devin flag still needs its
  contents recovered before merge, regardless of green check status.
