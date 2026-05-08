<script lang="ts">
  import type { GifItem } from "../../lib/gifs";

  type Props = {
    gifs: GifItem[];
    query: string;
    onQuery: (value: string) => void;
    onPick: (url: string, title: string) => void;
  };

  let { gifs, query, onQuery, onPick }: Props = $props();
</script>

<section class="gif-picker" aria-label="GIF picker panel">
  <div class="gif-picker-head">
    <strong>GIFs</strong>
    <input
      value={query}
      placeholder="Search reactions"
      aria-label="Search GIFs"
      oninput={(event) => onQuery(event.currentTarget.value)}
    />
  </div>
  <div class="gif-grid">
    {#each gifs as gif (gif.url)}
      <button type="button" onclick={() => onPick(gif.url, gif.title)}>
        <img src={gif.url} alt={gif.title} loading="lazy" />
        <span>{gif.title}</span>
      </button>
    {/each}
  </div>
</section>
