<script lang="ts">
  type Props = {
    direction: "older" | "newer";
    rows?: number;
  };

  let { direction, rows = 3 }: Props = $props();
</script>

<div
  class={`history-loader history-loader--${direction}`}
  role="status"
  aria-label={direction === "older" ? "Loading older messages" : "Loading newer messages"}
>
  {#each Array.from({ length: rows }) as _, index}
    <div class="history-loader__row" style={`--history-row: ${index}`}>
      <div class="history-loader__avatar"></div>
      <div class="history-loader__body">
        <span class="history-loader__meta"></span>
        <span class="history-loader__line history-loader__line--wide"></span>
        <span class="history-loader__line history-loader__line--mid"></span>
      </div>
    </div>
  {/each}
</div>
