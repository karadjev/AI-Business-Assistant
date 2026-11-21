import "server-only";

export function apiBaseURL(): string {
 const raw = process.env.API_BASE_URL ?? "http://127.0.0.1:8080";
 const url = new URL(raw);
 if (!["http:","https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash || url.pathname !== "/") {
  throw new Error("Invalid server API configuration");
 }
 return url.origin;
}
