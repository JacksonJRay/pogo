import { json } from "@sveltejs/kit";
import { report } from "$lib/report";

export const prerender = false;

export function GET() {
  const body = report();
  return json(body, { status: body.ok ? 200 : 500 });
}
