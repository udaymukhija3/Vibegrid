import path from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = path.dirname(fileURLToPath(import.meta.url));
const goBackendUrl = process.env.GO_BACKEND_URL ?? "http://127.0.0.1:8081";

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  outputFileTracingRoot: projectRoot,
  async rewrites() {
    return {
      beforeFiles: [
        {
          source: "/api/:path*",
          destination: `${goBackendUrl}/api/:path*`
        }
      ]
    };
  }
};

export default nextConfig;
