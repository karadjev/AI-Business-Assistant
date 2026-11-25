"use client";

import {useId, useState} from "react";
import {money} from "@/lib/catalog";

const measures = {
  revenue_minor: {label:"Выручка", explanation:"Стоимость завершённых услуг по месяцу внесения записи."},
  cash_minor: {label:"Поступления", explanation:"Подтверждённая оплата по месяцу её регистрации."},
  cost_minor: {label:"Себестоимость", explanation:"Стоимость деталей по месяцу внесения записи."},
  gross_minor: {label:"Валовая прибыль", explanation:"Стоимость услуг за вычетом стоимости деталей по месяцу внесения записи."},
} as const;

type Measure = keyof typeof measures;
export type FinancePeriod = {month:string} & Record<Measure,number|null>;
const keys = Object.keys(measures) as Measure[];
const comparisons = {
  income:["revenue_minor","cash_minor"],
  margin:["cost_minor","gross_minor"],
} as const satisfies Record<string,readonly Measure[]>;
type Comparison = keyof typeof comparisons;

const shortMonth = new Intl.DateTimeFormat("ru-RU",{month:"short",year:"2-digit",timeZone:"UTC"});
const monthLabel = (month:string) => shortMonth.formatToParts(date(month))
  .filter(part => part.type === "month" || part.type === "year").map(part => part.value).join("\u00a0");
const fullMonth = new Intl.DateTimeFormat("ru-RU",{month:"long",year:"numeric",timeZone:"UTC"});
function date(month:string) {return new Date(`${month}-01T00:00:00Z`);}

export function WorkshopFinanceChart({periods = [], timezone}: {periods?:FinancePeriod[]; timezone:string}) {
  const title = useId();
  const detail = useId();
  const [range,setRange] = useState(6);
  const [comparison,setComparison] = useState<Comparison>("income");
  const [selected,setSelected] = useState<string>();
  const valid = periods.filter(row => /^\d{4}-(0[1-9]|1[0-2])$/.test(row.month)
    && keys.every(key => row[key] === null || Number.isSafeInteger(row[key])));
  const rows = valid.slice(-range);
  const series = comparisons[comparison];
  const values = rows.flatMap(row => series.map(key => row[key] ?? 0));
  const min = Math.min(0,...values);
  const max = Math.max(0,...values);
  const span = max-min || 1;
  const zero = -min/span*100;
  const current = rows.find(row => row.month === selected) ?? rows.filter(row => series.some(key => row[key] !== null)).at(-1) ?? rows.at(-1);
  const hasData = rows.some(row => series.some(key => row[key] !== null));

  return <section className="panel finance-panel" id="workshop-finance" aria-labelledby={title}>
    <div className="finance-heading">
      <div><h3 id={title}>Финансы автосервиса</h3><p>Учтённые заявки · рубли · {timezone}</p></div>
      <div className="finance-controls">
        <label>Показатели<select value={comparison} onChange={event => setComparison(event.target.value as Comparison)}>
          <option value="income">Выручка и поступления</option><option value="margin">Себестоимость и валовая прибыль</option>
        </select></label>
        <label>Период<select value={range} onChange={event => setRange(Number(event.target.value))}>
          <option value={3}>3 месяца</option><option value={6}>6 месяцев</option><option value={12}>12 месяцев</option>
        </select></label>
      </div>
    </div>
    {valid.length !== periods.length && <p role="status">Часть значений вне поддерживаемого диапазона.</p>}
    {!hasData && <p className="finance-empty" role="status">{valid.length ? "За этот период нет записей по выбранным показателям." : "Помесячные данные пока недоступны."}</p>}
    {hasData && current && <>
      <ul className="finance-legend">{series.map((key,i) => <li key={key}><span className={`finance-series-${i}`}/>{measures[key].label}</li>)}</ul>
      <div className="finance-plot" style={{"--finance-zero":`${zero}%`} as React.CSSProperties} role="group" aria-label="Показатели по месяцам">
        {rows.map(row => <button key={row.month} type="button" aria-pressed={current.month === row.month} aria-controls={detail}
          aria-label={`${fullMonth.format(date(row.month))}. ${series.map(key => `${measures[key].label} ${row[key] === null ? "нет записи" : money(row[key])}`).join(". ")}`}
          onClick={() => setSelected(row.month)} onFocus={() => setSelected(row.month)} onPointerEnter={event => {if (event.pointerType === "mouse") setSelected(row.month);}}>
          <span>{monthLabel(row.month)}</span>
          <span className="finance-track" aria-hidden="true">{series.map((key,i) => {
            const value = row[key];
            return <span className="finance-lane" key={key}>{value !== null && <i className={`finance-series-${i}`}
              style={{left:`${(Math.min(0,value)-min)/span*100}%`,width:value === 0 ? "2px" : `${Math.abs(value)/span*100}%`}}/>}</span>;
          })}</span>
        </button>)}
        <div className="finance-scale" aria-hidden="true"><span>{money(min)}</span>{max !== min && <span>{money(max)}</span>}</div>
      </div>
      <div className="finance-detail" id={detail}>
        <h4>{fullMonth.format(date(current.month))}</h4>
        <dl>{series.map(key => <div key={key}><dt>{measures[key].label}</dt><dd>{current[key] === null ? "Нет записи" : money(current[key])}</dd></div>)}</dl>
      </div>
    </>}
    <details className="finance-source"><summary>Как рассчитаны показатели</summary>
      <ul>{series.map(key => <li key={key}><strong>{measures[key].label}. </strong>{measures[key].explanation}</li>)}</ul>
      <p>Месяцы без записей показаны без столбцов. Текущий месяц включает записи по сегодняшний день.</p>
    </details>
  </section>;
}
