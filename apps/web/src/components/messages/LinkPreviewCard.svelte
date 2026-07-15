<script lang="ts">
  import { firstPreviewURL, loadLinkPreview } from "../../lib/linkPreviews";
  import type { LinkPreview } from "../../lib/types";

  type Props = {
    body: string;
  };

  let { body }: Props = $props();
  let preview = $state<LinkPreview | null>(null);
  let imageVisible = $state(true);

  $effect(() => {
    const url = firstPreviewURL(body);
    preview = null;
    imageVisible = true;
    if (!url) return;
    let cancelled = false;
    loadLinkPreview(url).then((result) => {
      if (!cancelled) preview = result;
    });
    return () => {
      cancelled = true;
    };
  });
</script>

{#if preview}
  <a
    class="link-preview-card"
    class:without-image={!preview.image_url || !imageVisible}
    href={preview.url}
    target="_blank"
    rel="noreferrer noopener"
    aria-label={`Open link preview: ${preview.title || preview.site_name}`}
  >
    {#if preview.image_url && imageVisible}
      <img
        class="link-preview-image"
        src={preview.image_url}
        alt=""
        loading="lazy"
        onerror={() => (imageVisible = false)}
      />
    {/if}
    <span class="link-preview-copy">
      <span class="link-preview-site">{preview.site_name}</span>
      <strong>{preview.title || preview.url}</strong>
      {#if preview.description}<span class="link-preview-description">{preview.description}</span>{/if}
    </span>
  </a>
{/if}
