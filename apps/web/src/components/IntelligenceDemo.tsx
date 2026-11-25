"use client";

import {useLayoutEffect, useRef, useState} from "react";
import {Icon} from "./ProductUI";

const scenarios = [
  {
    name:"Анализ бизнеса",
    task:"Что требует моего внимания на этой неделе?",
    team:["Business Manager", "Аналитика", "Финансы"],
    title:"Обзор недели",
    facts:[["Новые заявки","12"],["Ожидают подтверждения","3"],["Поступления","86 400 ₽"]],
    note:"Подтвердите три заявки, чтобы команда могла продолжить работу. Для расчёта валовой прибыли сначала проверьте себестоимость.",
  },
  {
    name:"Заявка и остатки",
    task:"Проверь наличие двух настольных ламп и подготовь заявку.",
    team:["Business Manager", "Продажи", "Каталог"],
    title:"Черновик заявки",
    facts:[["Товар","Лампа «Север»"],["По каталогу","5 шт."],["Запрошено","2 шт."]],
    note:"В рабочем сценарии подтверждение передаёт заявку на обработку с указанным количеством. В демо остатки не изменятся.",
    approval:"Подтвердить заявку в демо",
  },
  {
    name:"Маркетинговое предложение",
    task:"Подготовь предложение для клиентов, которые давно не обращались.",
    team:["Business Manager", "Аналитика", "Маркетинг"],
    title:"Черновик предложения",
    facts:[["Аудитория","Клиенты без обращений 60 дней"],["Формат","Короткое личное сообщение"]],
    note:"«Здравствуйте! Мы обновили подборку настольного освещения. Если выбираете лампу для работы или дома, поможем сравнить варианты.»",
    approval:"Согласовать черновик в демо",
  },
] as const;

export function IntelligenceDemo() {
  const [selected, setSelected] = useState(0);
  const [approved, setApproved] = useState(false);
  const confirmation = useRef<HTMLElement>(null);
  useLayoutEffect(() => {if (approved) confirmation.current?.focus();},[approved]);
  const scenario = scenarios[selected];
  return <div className="intelligence-demo">
    <div className="demo-controls" aria-label="Демонстрационные сценарии">
      {scenarios.map((entry,index) => <button key={entry.name} type="button" aria-pressed={selected===index} onClick={() => {setSelected(index);setApproved(false);}}>{entry.name}</button>)}
    </div>
    <div className="demo-workspace">
      <div className="demo-instructions">
        <div className="demo-mode"><span/>Синтетические данные</div>
        <span className="demo-caption">Ваше поручение</span>
        <p className="demo-question">{scenario.task}</p>
        <span className="demo-caption">Участвуют</span>
        <ul className="demo-agents">{scenario.team.map(name => <li key={name}><Icon name="ai"/>{name}</li>)}</ul>
      </div>
      <section className="demo-result" aria-labelledby="demo-result-title" aria-live="polite">
        <span className="demo-caption"><Icon name="check"/> Результат</span>
        <h3 id="demo-result-title">{scenario.title}</h3>
        <dl>{scenario.facts.map(([name,value]) => <div key={name}><dt>{name}</dt><dd>{value}</dd></div>)}</dl>
        <p>{scenario.note}</p>
        {"approval" in scenario && (approved
          ? <strong tabIndex={-1} ref={confirmation} role="status" className="demo-confirmed">Согласовано в демо. Внешних действий нет.</strong>
          : <button type="button" className="button primary" onClick={() => setApproved(true)}>{scenario.approval}</button>)}
      </section>
    </div>
  </div>;
}
