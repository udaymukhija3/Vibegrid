import { spawn } from "node:child_process";

const children = [];
let shuttingDown = false;

function run(label, command, args, env = {}) {
  const child = spawn(command, args, {
    env: {
      ...process.env,
      ...env
    },
    stdio: "inherit"
  });

  children.push(child);
  child.on("exit", (code, signal) => {
    if (shuttingDown) {
      return;
    }

    shuttingDown = true;
    for (const process of children) {
      if (process !== child) {
        process.kill("SIGINT");
      }
    }

    if (signal) {
      console.error(`${label} exited with signal ${signal}`);
      process.exit(1);
    }

    process.exit(code ?? 0);
  });
}

function shutdown() {
  shuttingDown = true;
  for (const child of children) {
    child.kill("SIGINT");
  }
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

run("backend", "npm", ["run", "dev:backend"], {
  VIBEGRID_ADDR: process.env.VIBEGRID_ADDR ?? ":8081"
});

run("web", "npm", ["run", "dev:web"], {
  GO_BACKEND_URL: process.env.GO_BACKEND_URL ?? "http://127.0.0.1:8081"
});
