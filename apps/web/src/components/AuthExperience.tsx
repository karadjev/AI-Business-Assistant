import {IntelligenceCore} from "./IntelligenceCore";

export function AuthExperience({title, description, children}: {title:string; description?:string; children:React.ReactNode}) {
  return <main id="main" tabIndex={-1} className="auth-experience">
    <div className="auth-art">
      <p className="intelligence-kicker"><span/> The Intelligence</p>
      <IntelligenceCore/>
      <p className="auth-art-title">Ваш бизнес.<br/>Ваша AI-команда.</p>
      <p>Семь специалистов в одном рабочем пространстве.</p>
    </div>
    <div className="auth-panel"><div className="auth-content">
      <h1>{title}</h1>
      {description && <p className="lead">{description}</p>}
      {children}
    </div></div>
  </main>;
}
