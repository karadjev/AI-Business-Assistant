"use client";
export default function ErrorPage({reset}:{reset:()=>void}){return <main id="main" tabIndex={-1} className="status-page"><h1>Не удалось загрузить страницу</h1><p>Повторите попытку. Если ошибка сохраняется, проверьте запуск сервисов.</p><button onClick={reset}>Повторить</button></main>}
