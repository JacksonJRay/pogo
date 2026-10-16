const KEYS = [
  "PORT",
  "HOST",
  "DATABASE_URL",
  "API_KEY",
  "JWT_SECRET",
  "QUOTED_VALUE",
  "SPACED_VALUE",
  "EQUALS_VALUE",
  "MULTILINE_VALUE",
  "VITE_PUBLIC_TITLE",
  "NEXT_PUBLIC_TITLE",
];

let failed = false;
for (const key of KEYS) {
  const value = process.env[key];
  if (value === undefined) {
    console.log(`POGO_ENV\t${key}\tMISSING`);
    failed = true;
    continue;
  }
  console.log(`POGO_ENV\t${key}\t${Buffer.from(value, "utf8").toString("hex")}`);
}
process.exit(failed ? 1 : 0);
