export default function App() {
  const title = import.meta.env.VITE_PUBLIC_TITLE;
  return (
    <main style={{ maxWidth: "40rem", margin: "0 auto", padding: "2.5rem 1.25rem", fontFamily: "Palatino, serif" }}>
      <p style={{ letterSpacing: "0.14em", textTransform: "uppercase", fontFamily: "ui-monospace, monospace", fontSize: 12 }}>
        Pogo · Vite
      </p>
      <h1>Client bundle sees VITE_ names.</h1>
      <p>
        VITE_PUBLIC_TITLE: <code data-vite-title={title}>{title}</code>
      </p>
      <p>
        Other variables stay on the dev server. See <a href="/env">/env</a>.
      </p>
    </main>
  );
}
