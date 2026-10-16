export const metadata = { title: "Pogo Next.js example" };

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body style={{ margin: 0, background: "#f6f3ee", color: "#1c1915", fontFamily: "Palatino, serif" }}>
        {children}
      </body>
    </html>
  );
}
