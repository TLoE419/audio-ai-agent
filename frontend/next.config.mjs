import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const appRoot = dirname(fileURLToPath(import.meta.url));
const backendUrl = process.env.BACKEND_URL ?? "http://127.0.0.1:18080";

/** @type {import("next").NextConfig} */
const nextConfig = {
  turbopack: {
    root: appRoot,
  },
  async rewrites() {
    return [
      {
        source: "/backend/:path*",
        destination: `${backendUrl}/:path*`,
      },
    ];
  },
};

export default nextConfig;
