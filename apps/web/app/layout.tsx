import type { Metadata, Viewport } from "next";
import { Literata, Syne } from "next/font/google";
import "./globals.css";

const sans = Syne({
  variable: "--font-syne",
  subsets: ["latin"],
  weight: ["500", "600", "700", "800"],
  display: "swap",
});

const serif = Literata({
  variable: "--font-literata",
  subsets: ["latin"],
  display: "swap",
});

export const metadata: Metadata = {
  title: "Aperture",
  description: "NSE positional cash paper book versus Nifty. Fills only while the session is open.",
};

export const viewport: Viewport = {
  themeColor: "#c5cad3",
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={`${sans.variable} ${serif.variable} font-sans antialiased`}>{children}</body>
    </html>
  );
}
