import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "CT-Flow · Connected Tech",
  description: "Visual-prompt labeling for manufacturing vision datasets.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Middlefront typography. Intranet deployments fall back to system fonts. */}
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700;800&family=Noto+Sans+Thai:wght@400;500;600;700&display=swap"
        />
      </head>
      <body>
        <script dangerouslySetInnerHTML={{ __html: 'try{document.documentElement.dataset.theme=localStorage.getItem("ctflow.theme")==="dark"?"dark":"light"}catch{}' }} />
        {/* 2.4.1 — the app bar repeats on every page and is the first thing in
            the tab order. This is the way past it; it is invisible until it
            takes focus. */}
        <a className="skip" href="#main">Skip to main content</a>
        {children}
      </body>
    </html>
  );
}
