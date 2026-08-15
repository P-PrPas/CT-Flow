import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "CT-Flow · Connected Tech",
  description: "Visual-prompt labeling for manufacturing vision datasets.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        {/* Poppins + IBM Plex Sans Thai are the connectedtech.co.th faces. Loaded
            by <link> rather than next/font so an intranet build with no access
            to fonts.googleapis.com degrades to the fallback stack instead of
            failing the build. */}
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Poppins:wght@400;500;600;700&family=IBM+Plex+Sans+Thai:wght@400;500;600&display=swap"
        />
      </head>
      <body>{children}</body>
    </html>
  );
}
