<script lang="ts">
  import ChatApp from "./ChatApp.svelte";
  import ProductSite from "./ProductSite.svelte";

  const path = window.location.pathname;
  const isAppHost = window.location.hostname.startsWith("app.");
  const isApp = isAppHost || path === "/app" || path.startsWith("/app/");
</script>

{#if isApp}
  <ChatApp />
{:else}
  <ProductSite />
{/if}
