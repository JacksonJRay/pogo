<script>
  export let data;
</script>

<svelte:head>
  <title>Pogo shared env · SvelteKit</title>
</svelte:head>

<main>
  <p class="stamp">Pogo · SvelteKit · same file</p>
  <h1>SvelteKit received the shared environment.</h1>
  <p class="status">
    {data.ok ? "Every variable from the shared file is set." : "Some variables are missing."}
  </p>
  <p>
    Values are masked (first two and last two characters). This server uses
    <code>$env/dynamic/private</code>, so the variables are read when the page runs.
    Flask on the host and the Node container read the same <code>.env.enc</code>.
  </p>
  <ul>
    {#each data.rows as row}
      <li class:missing={!row.present}>
        <span class="key">{row.key}</span>
        <span class="val">{row.masked}</span>
      </li>
    {/each}
  </ul>
  <p>Machine-readable hashes: <a href="/env">/env</a>.</p>
</main>

<style>
  :global(body) {
    margin: 0;
    background: #f4f0e6;
    color: #1c1915;
  }
  main {
    max-width: 36rem;
    margin: 0 auto;
    padding: 2.5rem 1.25rem 3rem;
    font: 17px/1.45 Palatino, "Iowan Old Style", serif;
  }
  .stamp {
    margin: 0;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    font: 12px ui-monospace, monospace;
    color: #c4450a;
  }
  h1 {
    font-size: 2.1rem;
    line-height: 1.15;
    margin: 0.4rem 0 0.6rem;
  }
  p {
    margin: 0.4rem 0;
  }
  .status {
    color: #c4450a;
    font-family: ui-monospace, monospace;
    font-size: 13px;
  }
  ul {
    list-style: none;
    padding: 0;
    margin: 1.25rem 0 0;
  }
  li {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    border-top: 1px solid #ddd4c4;
    padding: 0.45rem 0;
    font-family: ui-monospace, monospace;
    font-size: 14px;
  }
  .missing .val {
    color: #9a3412;
  }
  a {
    color: #c4450a;
  }
</style>
