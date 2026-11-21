import {StatusIndicator} from "@/components/StatusIndicator";
export const metadata={title:"Состояние системы"};
export default function Status(){return <main id="main" tabIndex={-1} className="status-page"><span className="eyebrow">Диагностика</span><h1>Состояние системы</h1><p className="lead">Результат реального запроса к backend. Без демонстрационных значений.</p><StatusIndicator/><div className="notice"><strong>Что проверяется</strong><p>Доступность HTTP API и его версия. Готовность PostgreSQL проверяется отдельно через /health/ready.</p></div></main>}
