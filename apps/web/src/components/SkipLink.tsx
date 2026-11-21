"use client";
import {useCallback,useEffect,useRef} from "react";
export function SkipLink(){
 const observer=useRef<MutationObserver|null>(null);
 const move=useCallback(()=>{
  observer.current?.disconnect();
  function ready(){
   if(location.hash!=="#main")return true;
   const main=document.getElementById("main");
   // Streamed loading landmarks are replaced; focus the visible final content.
   if(!main||main.getAttribute("aria-busy")==="true"||main.getAttribute("role")==="status"||!main.getClientRects().length)return false;
   main.focus({preventScroll:true});main.scrollIntoView({block:"start"});return true;
  }
  if(ready())return;
  const watch=new MutationObserver(()=>{if(ready())watch.disconnect()});
  observer.current=watch;watch.observe(document.body,{childList:true,subtree:true,attributes:true,attributeFilter:["hidden","aria-busy"]});
 },[]);
 useEffect(()=>{const changed=()=>{if(location.hash==="#main")move();else observer.current?.disconnect()};changed();window.addEventListener("hashchange",changed);return()=>{window.removeEventListener("hashchange",changed);observer.current?.disconnect()}},[move]);
 return <a className="skip-link" href="#main" onClick={event=>{event.preventDefault();history.replaceState(null,"",location.pathname+location.search+"#main");move()}}>Перейти к содержимому</a>
}
