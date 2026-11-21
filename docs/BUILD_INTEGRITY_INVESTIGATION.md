# Исследование E01 LOW — этап 8Е

Статус: **OPEN**. Текущий повтор сборки/запуска прошёл; историческое происхождение каталогов не доказано. Никаких исключений для каталогов не добавлено.

## Установленные факты

Исторические пути относительно репозитория:

- `.local/build-env/web/.next/standalone/.next 2`
- `.local/build-env/web/.next/standalone/node_modules 2`

Они отсутствуют при начале 8Е. В 8Д они уже удалены как пустые; сохранился только `.local/build-env/phase2/artifact-empty-directory-observation.json`. Там записаны отсутствие изменённых runtime files и совпадение первоначального standalone hash после исключения ровно этих пустых директорий. Первоначальный proof не регенерировался. Это историческое свидетельство, а не повторная экспертиза удалённых inode; времени рождения, PID и журнала создания нет.

Успешный multichannel browser прогон 8Д закончился около 2026-10-21T18:13:36.511Z. mtime JSON наблюдения — 18:17:20.209896Z; после исправления backend-down стартовал в 18:17:27.186Z. Это границы хронологии по сохранённым отчётам/метаданным, **не точное время создания каталогов**. Из них нельзя установить виновный шаг или процесс. Сам mtime файла отчёта может меняться и не является журналом файловых событий.

Проверены `scripts/build-env/production.sh`, `browser.sh`, `artifacts.cjs` и browser specs. Production script копирует static/public в стандартные пути; browser wrapper проверяет proof и запускает Playwright. В проверенном коде нет команды создания имён `.next 2`/`node_modules 2`. Их суффикс не является доказательством участия Finder, Docker, сборщика или пользователя. Нет оснований приписать происхождение какому-либо из них.

## Почему guard сработал

`standaloneDigest()` в `scripts/build-env/artifacts.cjs` включает каждый путь (включая директорию), символьные ссылки и содержимое файлов. Даже пустая дополнительная директория меняет digest. `verify()` сравнивает его с proof и блокирует запуск при несовпадении. Такое поведение ожидаемо и сохранено. Source digest имеет отдельную область применения; исключения source не применяются к runtime node_modules.

В `scripts/build-env/artifacts.test.cjs` добавлена отрицательная регрессия: для **обоих точных имён** отдельно проверяется отказ при пустом каталоге и при файле внутри него; после удаления тестового изменения первоначальный proof снова проходит. Проверки stale source и изменённого server.js сохранены. Test fixture находится в temporary directory контейнера, реальные artifacts не модифицируются этим тестом. PASS, 1 расширенный automated test.

## Повтор 8Е

До запуска сохранён manifest 2633 записей с paths/kinds/SHA256/mtime/ctime. Выполнен fresh production build в прежнем Linux/amd64 workspace без installs/downloads. Новый BUILD_ID `L2Asvut-COg-ns9-CXGPg`, proof UTC `2026-10-21T18:26:56.898Z`; source SHA256 `0259af2e7fa1d1ca57a30cfd3c00e8edc7d51ebdafcc45024111435f21202535` совпадает с 8Д.

Между разными сборками новый build ID, build manifests и сгенерированные Next HTML/RSC/preview/reference данные меняются: 3 пути добавлены, 3 удалены, 45 файлов с иными bytes. Все изменения относятся к generated `.next`; server.js/required-server-files hash сохранены, dependency files/paths не изменились. Это **не заявление о побайтовой воспроизводимости Next build**. Полного старого содержимого generated файлов не сохранено для семантического сравнения каждой строки. Доказана повторная успешная сборка тех же inputs и стабильность нового artifact после его создания.

После build сохранён новый manifest и immutable proof; выполнены повторные verify, served-marker, local `/`, `/login`, `/register` HTTP200. Runtime mount RO, network internal, без host ports. После запуска: **все 2633 записи, bytes и записанные timestamps совпали, proof не менялся**. E01 пути отсутствуют, неожиданных мутаций нового artifact не обнаружено. Это ограниченный наблюдаемый интервал, не доказательство невозможности будущей мутации host bind mount.

Evidence `.local/audit8e/`: `standalone-before.json`, `standalone-after-build.json`, `standalone-after-runtime.json`, `rebuild-diff.json`, `runtime-integrity.json`, `build-proof.json`, `repeated-guard.log`, `guard-tests.log`, `local-served-guard.log`.

## Остаточный риск и дальнейшая диагностика

E01 остаётся LOW/OPEN: источник единичного host-backed staging изменения не установлен. Guard fail-closed снижает риск запуска неожиданного дерева. Новых CRITICAL/HIGH нарушений в выполненных проверках не обнаружено; это не исчерпывающий аудит всей системы.

При следующем воспроизведении не удалять пути до сохранения lstat/birthtime/ctime, дерева, proof и process evidence. Снимки делать перед/после copy, build, упаковки, каждого browser run и cleanup. Если нужны сведения о создателе, заранее отдельно согласовать доступный аудит файловых событий хоста; внутри контейнера он не покажет достоверно PID host-процесса. Для сравнения можно отдельным заданием провести тот же цикл на Docker-managed Linux volume. Не менять системные настройки и не переносить рабочие данные автоматически. До доказанной причины нельзя считать каталоги разрешённой мутацией или добавлять whitelist.
