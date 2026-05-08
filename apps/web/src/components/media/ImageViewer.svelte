<script lang="ts">
  type Props = {
    url: string;
    title: string;
    onClose: () => void;
  };

  let { url, title, onClose }: Props = $props();
</script>

<div class="modal-scrim image-viewer-scrim" role="presentation">
  <button class="modal-backdrop" type="button" aria-label="Close image viewer" onclick={onClose}></button>
  <section class="image-viewer" aria-label="Image viewer">
    <header>
      <strong>{title}</strong>
      <div>
        <a href={url} target="_blank" rel="noreferrer">Open original</a>
        <button type="button" aria-label="Close image viewer" onclick={onClose}>×</button>
      </div>
    </header>
    <div class="image-viewer-stage">
      <img src={url} alt={title} />
    </div>
  </section>
</div>
