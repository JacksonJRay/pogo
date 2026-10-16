import { building } from "$app/environment";
import { report } from "$lib/report";

const status = report();
if (!building && !status.ok) {
  console.error("missing required env: " + status.missing.join(", "));
  process.exit(1);
}

export async function handle({ event, resolve }) {
  return resolve(event);
}
