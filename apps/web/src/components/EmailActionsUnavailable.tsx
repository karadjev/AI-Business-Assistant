import Link from "next/link";

export function EmailActionsUnavailable() {
  return (
    <section className="notice">
      <p>Регистрация и восстановление доступа временно недоступны. Если у вас уже есть подтверждённый аккаунт, войдите в кабинет.</p>
      <Link prefetch={false} href="/login">Перейти ко входу</Link>
    </section>
  );
}
