import { report } from "$lib/report";

export function load() {
  const body = report();
  return {
    ok: body.ok,
    rows: Object.entries(body.vars).map(([key, item]) => ({
      key,
      masked: item.masked,
      present: item.present,
    })),
  };
}
