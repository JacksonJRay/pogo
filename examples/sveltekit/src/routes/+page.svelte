<script>
  export let data;
</script>

<svelte:head>
  <title>Pogo SvelteKit example</title>
</svelte:head>

<main>
  <p class="stamp">Pogo · SvelteKit</p>
  <h1>Private env, read on the server.</h1>
  <p>
    This page uses <code>$env/dynamic/private</code>. Values are masked.
    <a href="/env">/env</a> returns hashes for the test.
  </p>
  <ul>
    {#each data.rows as row}
      <li><code>{row.key}</code> <span>{row.masked}</span></li>
    {/each}
  </ul>
</main>

<style>
  main {
    max-width: 40rem;
    margin: 0 auto;
    padding: 2.5rem 1.25rem;
    font: 16px/1.5 "Iowan Old Style", Palatino, serif;
    color: #1c1915;
  }
  :global(body) {
    margin: 0;
    background: #f3f6f4;
  }
  h1 {
    font-size: 2rem;
    margin: 0 0 0.25rem;
  }
  .stamp {
    letter-spacing: 0.14em;
    text-transform: uppercase;
    font: 12px ui-monospace, monospace;
    color: #0b6b4a;
  }
  ul {
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    border-top: 1px solid #d5e0da;
    padding: 0.4rem 0;
    font-family: ui-monospace, monospace;
    font-size: 14px;
  }
</style>
