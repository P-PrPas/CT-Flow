import type { NextConfig } from "next";

// The browser only ever talks to this app; /api/* is proxied to FastAPI at
// runtime by app/api/[...path]/route.ts (API_URL env). No CORS, no
// NEXT_PUBLIC_* to keep in sync per deployment.
const nextConfig: NextConfig = {
  output: "standalone", // ships a self-contained server.js -> tiny runtime image
};

export default nextConfig;
