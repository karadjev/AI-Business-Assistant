import {ThemeSwitch} from "@/components/ThemeSwitch";
import {Suspense} from "react";
import {Breadcrumbs} from "@/components/Breadcrumbs";
import {requireUser} from "@/lib/session";
import {Sidebar} from "@/components/Sidebar";
import {AccountMenu} from "@/components/AccountMenu";
export default async function DashboardLayout({children}:{children:React.ReactNode}){const user=await requireUser();return <div className="workspace"><Suspense><Sidebar/></Suspense><div className="dashboard" style={{flex:1}}><div className="account-bar"><Suspense><Breadcrumbs/></Suspense><div className="account-profile"><AccountMenu name={user.display_name} email={user.email}/><ThemeSwitch/></div></div>{children}</div></div>}
