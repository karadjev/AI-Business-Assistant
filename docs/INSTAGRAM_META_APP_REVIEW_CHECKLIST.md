# Instagram — Meta App Review checklist

Статус: **META APP REVIEW PENDING**. Ни App Review, ни Business verification, ни доступ к конкретному приложению Meta не подтверждены.

- [ ] Завершить runtime integration и все offline release gates из test report.
- [ ] Создать/выбрать Meta Developer App; подтвердить доступность Instagram API with Instagram Login в Dashboard.
- [ ] Добавить Professional Business/Creator test account и роли разработчика/тестировщика.
- [ ] Настроить Instagram App ID/Secret в backend secret store после реализации runtime; не вводить пароль аккаунта в Avero.
- [ ] Подготовить публичные Privacy Policy URL и Data Deletion instructions/callback по требованиям Dashboard.
- [ ] Зафиксировать и зарегистрировать exact HTTPS OAuth redirect и HTTPS webhook URL; сейчас backend routes отсутствуют.
- [ ] Подтвердить permissions `instagram_business_basic`, `instagram_business_manage_messages` и все необходимые webhook subscriptions.
- [ ] Проверить identity, expiry, subscription confirmation и подлинный inbound event до статуса Connected.
- [ ] Уточнить необходимость Business verification для выбранного access/use case.
- [ ] Запросить Advanced Access/App Review для подключения чужих магазинов, когда требуется; Standard Access не считать разрешением обслуживать всех клиентов.
- [ ] Подготовить screencast: owner login → OAuth consent → подтверждённый account → Direct inbound → общий inbox → каталог/цены/остатки → handoff → consent/staff order → disconnect/delete.
- [ ] Показать tenant isolation, окно переписки и отказ отправки за его пределами; не демонстрировать AI через Human Agent exception.
- [ ] Оформить тестовый доступ для reviewer, инструкции, описание использования каждой permission и обработку удаления данных.
- [ ] Получить и сохранить фактическое решение Meta; только затем менять статус review.
- [ ] Получить отдельное разрешение на реальные подключения/отправки и последующее production развёртывание.

Требования конкретного Dashboard могут отличаться; это подготовительный список, не подтверждение одобрения. [Meta Instagram Login](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/business-login), [Meta Messaging](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/messaging-api).
