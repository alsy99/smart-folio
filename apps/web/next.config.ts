import type { NextConfig } from "next";

function gatewayOrigin(): string {
  return (process.env.GATEWAY_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
}

const nextConfig: NextConfig = {
  allowedDevOrigins: ["*.trycloudflare.com", "*.ngrok-free.app", "*.ngrok.app", "*.ngrok.io"],
  async rewrites() {
    return [{ source: "/gw/:path*", destination: `${gatewayOrigin()}/:path*` }];
  },
};

export default nextConfig;
