export default function Page() {
  const title = process.env.NEXT_PUBLIC_TITLE || "";
  return (
    <main style={{ maxWidth: "40rem", margin: "0 auto", padding: "2.5rem 1.25rem" }}>
      <p style={{ letterSpacing: "0.14em", textTransform: "uppercase", fontFamily: "ui-monospace, monospace", fontSize: 12 }}>
        Pogo · Next.js
      </p>
      <h1>Server env is loaded.</h1>
      <p>
        Public title (NEXT_PUBLIC_): <code data-public-title={title}>{title}</code>
      </p>
      <p>
        Private variables stay on the server. See <a href="/api/env">/api/env</a>.
      </p>
    </main>
  );
}
