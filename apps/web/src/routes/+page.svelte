<script lang="ts">
  import ChatApp from "../ChatApp.svelte";
  import ProductSite from "../ProductSite.svelte";

  const isAppHost = window.location.hostname.startsWith("app.");
</script>

{#if isAppHost}
  <ChatApp />
{:else}
  <ProductSite />
{/if}
