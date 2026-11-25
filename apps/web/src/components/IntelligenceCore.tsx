import {useId} from "react";

export const intelligenceTeam = [
  {id:"business_manager", name:"Business Manager", role:"Координирует команду и собирает результаты поручений."},
  {id:"sales", name:"Продажи", role:"Готовит ответы по товарам и помогает разобрать заявки."},
  {id:"customer", name:"Поддержка", role:"Уточняет детали обращений и предлагает следующий шаг."},
  {id:"product_inventory", name:"Каталог", role:"Сверяет карточки, цены и остатки по данным организации."},
  {id:"finance", name:"Финансы", role:"Готовит сводки и обозначает, каких данных не хватает."},
  {id:"marketing", name:"Маркетинг", role:"Готовит черновики предложений для проверки владельцем."},
  {id:"analytics", name:"Аналитика", role:"Собирает наблюдения и предлагает, что проверить в первую очередь."},
] as const;

const satellites = [[142,173],[283,87],[472,132],[502,328],[352,415],[147,346]] as const;

export function IntelligenceCore({compact = false}: {compact?: boolean}) {
  const id = useId();
  return <svg className={compact ? "intelligence-core compact" : "intelligence-core"} viewBox="0 0 640 500" aria-hidden="true">
    <defs>
      <radialGradient id={id+"halo"}><stop stopColor="#82b5ff" stopOpacity=".32"/><stop offset=".68" stopColor="#3d83f6" stopOpacity=".08"/><stop offset="1" stopColor="#3d83f6" stopOpacity="0"/></radialGradient>
      <linearGradient id={id+"orbit"} x2="1" y2="1"><stop stopColor="#a3c7f2" stopOpacity=".12"/><stop offset=".48" stopColor="#dbeaff" stopOpacity=".64"/><stop offset="1" stopColor="#82b5ff" stopOpacity=".14"/></linearGradient>
      <linearGradient id={id+"metal"} x1="0" x2="1" y1="0" y2="1"><stop stopColor="#f0f7ff"/><stop offset=".30" stopColor="#a3c7f2"/><stop offset=".58" stopColor="#496e9f"/><stop offset=".80" stopColor="#173354"/><stop offset="1" stopColor="#87b4fc"/></linearGradient>
      <linearGradient id={id+"facet"} x1="1" y1="0" x2="0" y2="1"><stop stopColor="#e7f3ff"/><stop offset=".48" stopColor="#6496ce"/><stop offset="1" stopColor="#173354"/></linearGradient>
      <radialGradient id={id+"node"} cx=".33" cy=".24" r=".74"><stop stopColor="#fff"/><stop offset=".32" stopColor="#b6d8fa"/><stop offset=".72" stopColor="#4873a3"/><stop offset="1" stopColor="#173454"/></radialGradient>
      <pattern id={id+"grid"} width="28" height="28" patternUnits="userSpaceOnUse"><path d="M0 28V0H28" fill="none" stroke="#a3c7f2" strokeOpacity=".10"/></pattern>
    </defs>
    <circle cx="320" cy="250" r="237" fill={`url(#${id}halo)`}/>
    <circle cx="320" cy="250" r="220" fill={`url(#${id}grid)`} opacity=".55"/>
    <g fill="none" stroke={`url(#${id}orbit)`}>
      <circle cx="320" cy="250" r="207" strokeDasharray="2 7"/>
      <ellipse cx="320" cy="250" rx="227" ry="91" transform="rotate(-26 320 250)"/>
      <ellipse cx="320" cy="250" rx="227" ry="101" transform="rotate(32 320 250)"/>
      <ellipse cx="320" cy="250" rx="180" ry="135" transform="rotate(76 320 250)"/>
      {satellites.map(([x,y]) => <path key={x} d={`M320 250L${x} ${y}`} strokeDasharray="3 7"/>)}
    </g>
    <circle cx="320" cy="250" r="100" fill="#0d2543" stroke="#a1c8ff" strokeOpacity=".46"/>
    <circle cx="320" cy="250" r="89" fill="none" stroke="#a3c7f2" strokeOpacity=".24" strokeDasharray="1 4"/>
    <g stroke="#e0efff" strokeOpacity=".55" strokeLinejoin="round">
      <path d="M320 169 386 219 369 296 320 333 271 296 254 219Z" fill={`url(#${id}metal)`}/>
      <path d="M320 169 344 224 386 219 320 250 254 219 296 224Z" fill={`url(#${id}facet)`}/>
      <path d="M320 250 386 219 369 296 320 333 344 276Z" fill={`url(#${id}facet)`}/>
      <path d="M320 250 296 224 320 169 344 224Z" fill="#e0f0ff" fillOpacity=".5"/>
      <path d="M320 250 344 276 320 333 296 276Z" fill="#173354" fillOpacity=".64"/>
    </g>
    {satellites.map(([x,y],i) => <g key={x}>
      <circle cx={x} cy={y} r={24} fill="#122d50" stroke="#82b5ff" strokeOpacity=".44"/>
      <circle cx={x} cy={y} r={i===3 ? 12 : 10} fill={`url(#${id}node)`} stroke="#a3c7f2" strokeOpacity=".7"/>
      <path d={`M${x-32} ${y}h-8m72 0h8`} stroke="#a3c7f2" strokeOpacity=".32"/>
    </g>)}
  </svg>;
}
