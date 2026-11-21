# Go race final — 8Д, фаза2

**PASS.** Реально выполнены instrumented Go unit tests и PostgreSQL integration suite с `-race`. Обычные CGO0 tests не использованы как замена.

## Toolchain и установка

Go1.26.2/linux/amd64, CGO_ENABLED=1, CC=gcc, GCC15.2.0-r2, musl-dev1.2.5-r23. Все13 APK ровно согласованных версий; signature verify и SHA256 каждого PASS. Компилятор установлен в отдельном контейнере с `--network none`, без изменения macOS/рабочих контейнеров. Standard busybox trigger установленного base package выполнился как часть APK installation; npm lifecycle scripts не включались.

Из установленного собственного контейнера получен rootfs export и **локальный import image**, без Docker registry lookup/Dockerfile network metadata и без git commit. Image `aba-go-race:go1.26.2-alpine3.23-local`, ID `sha256:37e9cbb11894d91192f6c38b9e97935b0577edc81160de608a083407d167d7e2`. Его ENV восстанавливает Go PATH/GOPATH/offline gates/CGO1. Это local test toolchain, не production deployment.

## Реально выполнено

- C headers/linker compile+run probe:PASS, `check-go.sh`.
- `go test -race -count=1 ./...`:PASS, весь релевантный единственный module apps/api, включая компиляцию test-only browserfixture helper. Data race warnings отсутствуют.
- `go test -race -tags=integration -count=1 -v ./tests/integration`:**PASS33.846s**.
- Обычные Go tests/vet/build API/worker/migrate/healthcheck + synthetic browserfixture:PASS. Fresh Go binaries использованы actual browser runtime.

Integration: clean/repeated/concurrent migrations,32 tenant-owned FORCE RLS tables под aba_app (не bypass/owner), foreign SELECT/INSERT/UPDATE/DELETE/WITH CHECK, tenant context cleanup,3-store2-channel backend matrix, invalid signatures/dedup/consent/no early reserve/concurrent last unit/price-stock conflicts/handoff, actual child-process crash/UNKNOWN/no resend/manual resolve/fencing/retry, conservative budget settlement/recovery/zero live gate. Child helper top-level SKIP — только способ запуска subprocess; parent crash tests выполнены.

## Изоляция/воспроизведение

Cached images pull never; internal Docker network IPv6=false, без usable external default route, host ports и docker socket, proxies unset, Go proxy/sumdb/toolchain downloads/telemetry off. Локальные HTTP provider emulators только loopback; настоящие provider APIs не вызывались.

Три Compose файла: infrastructure/compose.environment.yml, compose.environment-smoke.yml, compose.prepared-build.yml. Ignored env `.local/audit8d/compose.env` задаёт лишь локальный module cache. Commands:

```sh
docker compose --env-file .local/audit8d/compose.env -p aba-phase2-check -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm race
docker compose --env-file .local/audit8d/compose.env -p aba-phase2-check -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm integration
```

Integration требует чистую **собственную** aba_test DB; перед повтором удалять только свой project/его synthetic volume, не рабочую БД. Evidence `.local/audit8d/artifacts/race-final.log`, `integration-race.log`, `rls_inventory.json`; APK/установка — `.local/build-env/phase2/`. Состав/команды подробно в BUILD_ENVIRONMENT_PHASE_2_REPORT. Гонок в выполненных сценариях не обнаружено; это не математическое доказательство отсутствия всех возможных гонок.
