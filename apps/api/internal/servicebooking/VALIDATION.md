
## 2026-11-19T15:59:09.050415+00:00
`go test -count=1 -v ./internal/servicebooking` exit 0
```text
=== RUN   TestTimezoneGapFoldAndOvernight
=== RUN   TestTimezoneGapFoldAndOvernight/moscow
=== RUN   TestTimezoneGapFoldAndOvernight/dst-gap
=== RUN   TestTimezoneGapFoldAndOvernight/dst-fold-early-explicit
=== RUN   TestTimezoneGapFoldAndOvernight/dst-fold-late-explicit
=== RUN   TestTimezoneGapFoldAndOvernight/fold-offset-changes
=== RUN   TestTimezoneGapFoldAndOvernight/wrong-offset
=== RUN   TestTimezoneGapFoldAndOvernight/wrong-fold-offset
=== RUN   TestTimezoneGapFoldAndOvernight/local-host-forbidden
=== RUN   TestTimezoneGapFoldAndOvernight/unknown-zone
=== RUN   TestTimezoneGapFoldAndOvernight/utc
--- PASS: TestTimezoneGapFoldAndOvernight (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/moscow (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/dst-gap (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/dst-fold-early-explicit (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/dst-fold-late-explicit (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/fold-offset-changes (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/wrong-offset (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/wrong-fold-offset (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/local-host-forbidden (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/unknown-zone (0.00s)
    --- PASS: TestTimezoneGapFoldAndOvernight/utc (0.00s)
=== RUN   TestSelectionBoundsAndCanonicalIdempotency
--- PASS: TestSelectionBoundsAndCanonicalIdempotency (0.00s)
=== RUN   TestUnavailableDependenciesDoNotAuthorize
--- PASS: TestUnavailableDependenciesDoNotAuthorize (0.00s)
=== RUN   TestOfferingUnknownMoneyAndOverflow
--- PASS: TestOfferingUnknownMoneyAndOverflow (0.00s)
PASS
ok  	github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking	0.576s
```

## 2026-11-19T15:59:15.809505+00:00
`go test -race -count=1 ./internal/servicebooking` exit 0
```text
ok  	github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking	1.707s
```

## 2026-11-19T15:59:16.571629+00:00
`go vet ./internal/servicebooking` exit 0
```text
```

## 2026-11-19T15:59:16.871063+00:00
`go build ./internal/servicebooking` exit 0
```text
```
