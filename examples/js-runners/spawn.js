import { spawn } from "node:child_process";

const child = spawn(process.execPath, ["print-env.js"], { stdio: "inherit" });
child.on("exit", (code) => process.exit(code ?? 1));
