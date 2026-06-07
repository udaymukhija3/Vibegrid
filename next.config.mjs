import path from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = path.dirname(fileURLToPath(import.meta.url));
const goBackendUrl = process.env.GO_BACKEND_URL ?? "http://127.0.0.1:8081";
const isStaticExport = process.env.VIBEGRID_STATIC_EXPORT === "true";

const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Strict-Transport-Security", value: "max-age=63072000; includeSubDomains; preload" }
];

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  outputFileTracingRoot: projectRoot,
  images: {
    unoptimized: true
  },
  ...(isStaticExport
    ? {
        output: "export",
        trailingSlash: true
      }
    : {
        async headers() {
          return [{ source: "/:path*", headers: securityHeaders }];
        },
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
      })
};

export default nextConfig;
