import {useId} from "react";

export const intelligenceTeam = [
  {name:"Business Manager", role:"Координирует команду и собирает результаты поручений."},
  {name:"Продажи", role:"Готовит ответы по товарам и помогает разобрать заявки."},
  {name:"Поддержка", role:"Уточняет детали обращений и предлагает следующий шаг."},
  {name:"Каталог", role:"Сверяет карточки, цены и остатки по данным организации."},
  {name:"Финансы", role:"Готовит сводки и обозначает, каких данных не хватает."},
  {name:"Маркетинг", role:"Готовит черновики предложений для проверки владельцем."},
  {name:"Аналитика", role:"Собирает наблюдения и предлагает, что проверить в первую очередь."},
] as const;

const satellites = [[142,173],[283,87],[472,132],[502,328],[352,415],[147,346]] as const;

export function IntelligenceCore({compact = false}: {compact?: boolean}) {
  const id = useId();
  return <svg className={compact ? "intelligence-core compact" : "intelligence-core"} viewBox="0 0 640 500" aria-hidden="true">
    <defs>
      <radialGradient id={id+"halo"}><stop stopColor="#a9a2e8" stopOpacity=".32"/><stop offset=".68" stopColor="#8a7dff" stopOpacity=".08"/><stop offset="1" stopColor="#8a7dff" stopOpacity="0"/></radialGradient>
      <linearGradient id={id+"orbit"} x2="1" y2="1"><stop stopColor="#bbc3d6" stopOpacity=".12"/><stop offset=".48" stopColor="#daddff" stopOpacity=".64"/><stop offset="1" stopColor="#a9a2e8" stopOpacity=".14"/></linearGradient>
      <linearGradient id={id+"metal"} x1="0" x2="1" y1="0" y2="1"><stop stopColor="#f6f5f2"/><stop offset=".30" stopColor="#bbc3d6"/><stop offset=".58" stopColor="#6c6b8d"/><stop offset=".80" stopColor="#252636"/><stop offset="1" stopColor="#b0a9fa"/></linearGradient>
      <linearGradient id={id+"facet"} x1="1" y1="0" x2="0" y2="1"><stop stopColor="#e9eaff"/><stop offset=".48" stopColor="#8c88b6"/><stop offset="1" stopColor="#252636"/></linearGradient>
      <radialGradient id={id+"node"} cx=".33" cy=".24" r=".74"><stop stopColor="#fff"/><stop offset=".32" stopColor="#c0c4e2"/><stop offset=".72" stopColor="#666987"/><stop offset="1" stopColor="#242533"/></radialGradient>
      <pattern id={id+"grid"} width="28" height="28" patternUnits="userSpaceOnUse"><path d="M0 28V0H28" fill="none" stroke="#bbc3d6" strokeOpacity=".10"/></pattern>
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
    <circle cx="320" cy="250" r="100" fill="#0d0e17" stroke="#bbb4ff" strokeOpacity=".46"/>
    <circle cx="320" cy="250" r="89" fill="none" stroke="#bbc3d6" strokeOpacity=".24" strokeDasharray="1 4"/>
    <g stroke="#dce0ef" strokeOpacity=".55" strokeLinejoin="round">
      <path d="M320 169 386 219 369 296 320 333 271 296 254 219Z" fill={`url(#${id}metal)`}/>
      <path d="M320 169 344 224 386 219 320 250 254 219 296 224Z" fill={`url(#${id}facet)`}/>
      <path d="M320 250 386 219 369 296 320 333 344 276Z" fill={`url(#${id}facet)`}/>
      <path d="M320 250 296 224 320 169 344 224Z" fill="#e6e5f7" fillOpacity=".5"/>
      <path d="M320 250 344 276 320 333 296 276Z" fill="#202133" fillOpacity=".64"/>
    </g>
    {satellites.map(([x,y],i) => <g key={x}>
      <circle cx={x} cy={y} r={24} fill="#10111c" stroke="#a9a2e8" strokeOpacity=".44"/>
      <circle cx={x} cy={y} r={i===3 ? 12 : 10} fill={`url(#${id}node)`} stroke="#bbc3d6" strokeOpacity=".7"/>
      <path d={`M${x-32} ${y}h-8m72 0h8`} stroke="#bbc3d6" strokeOpacity=".32"/>
    </g>)}
  </svg>;
}
