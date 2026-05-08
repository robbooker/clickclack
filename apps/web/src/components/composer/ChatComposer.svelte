<script lang="ts">
  import { autoGrow } from "../../lib/actions/autogrow";
  import { formatBytes, isImageUpload, uploadURL } from "../../lib/uploads";
  import type { GifItem } from "../../lib/gifs";
  import type { Message, Upload } from "../../lib/types";
  import ComposerToolbar from "./ComposerToolbar.svelte";
  import GifPicker from "./GifPicker.svelte";
  import ReplyPreview from "./ReplyPreview.svelte";

  type Props = {
    value: string;
    placeholder: string;
    ariaLabel: string;
    submitLabel: string;
    formClass?: string;
    pendingUpload?: Upload | null;
    replyTarget?: Message | null;
    showUpload?: boolean;
    showToolbar?: boolean;
    showGifPicker?: boolean;
    gifQuery?: string;
    filteredGifs?: GifItem[];
    onValue: (value: string) => void;
    onSubmit: () => void;
    onKeydown: (event: KeyboardEvent) => void;
    onFocus: () => void;
    onInputRef: (node: HTMLTextAreaElement | null) => void;
    onUploadFile?: (event: Event) => void;
    onRemoveUpload?: () => void;
    onClearReply?: () => void;
    onApplyMarkdownWrap?: (before: string, after?: string) => void;
    onAppendToComposer?: (snippet: string) => void;
    onToggleGif?: () => void;
    onGifQuery?: (value: string) => void;
    onPickGif?: (url: string, title: string) => void;
  };

  let {
    value,
    placeholder,
    ariaLabel,
    submitLabel,
    formClass = "composer",
    pendingUpload = null,
    replyTarget = null,
    showUpload = false,
    showToolbar = false,
    showGifPicker = false,
    gifQuery = "",
    filteredGifs = [],
    onValue,
    onSubmit,
    onKeydown,
    onFocus,
    onInputRef,
    onUploadFile = () => {},
    onRemoveUpload = () => {},
    onClearReply = () => {},
    onApplyMarkdownWrap = () => {},
    onAppendToComposer = () => {},
    onToggleGif = () => {},
    onGifQuery = () => {},
    onPickGif = () => {},
  }: Props = $props();

  let input: HTMLTextAreaElement | null = $state(null);

  $effect(() => {
    onInputRef(input);
    return () => onInputRef(null);
  });
</script>

<form
  class={formClass}
  onsubmit={(event) => {
    event.preventDefault();
    onSubmit();
  }}
>
  {#if showGifPicker}
    <GifPicker
      gifs={filteredGifs}
      query={gifQuery}
      onQuery={onGifQuery}
      onPick={onPickGif}
    />
  {/if}
  <div class="composer-card">
    {#if pendingUpload}
      <div class="composer-attachment">
        <span class="attachment-icon" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="14" height="14"><path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M21.44 11.05 12.5 20a6 6 0 0 1-8.49-8.49l8.49-8.48a4 4 0 0 1 5.66 5.66l-8.49 8.49a2 2 0 0 1-2.83-2.83L13.41 7.5"/></svg>
        </span>
        {#if isImageUpload(pendingUpload)}
          <img class="pending-image" src={uploadURL(pendingUpload)} alt={pendingUpload.filename} />
        {/if}
        <span class="attachment-name">{pendingUpload.filename} · {formatBytes(pendingUpload.byte_size)}</span>
        <button type="button" class="attachment-remove" aria-label="Remove attachment" onclick={onRemoveUpload}>×</button>
      </div>
    {/if}
    {#if replyTarget}
      <ReplyPreview target={replyTarget} onClear={onClearReply} />
    {/if}
    <div class="composer-row">
      {#if showUpload}
        <label class="composer-icon" title="Upload file">
          <input type="file" aria-label="Upload file" onchange={onUploadFile} />
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M21.44 11.05 12.5 20a6 6 0 0 1-8.49-8.49l8.49-8.48a4 4 0 0 1 5.66 5.66l-8.49 8.49a2 2 0 0 1-2.83-2.83L13.41 7.5"/>
          </svg>
        </label>
      {/if}
      <textarea
        bind:this={input}
        value={value}
        use:autoGrow={value}
        rows="1"
        {placeholder}
        aria-label={ariaLabel}
        oninput={(event) => onValue(event.currentTarget.value)}
        onfocus={onFocus}
        onkeydown={onKeydown}
      ></textarea>
      <button type="submit" class="send" aria-label={submitLabel} disabled={!value.trim()}>
        <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
          <path fill="currentColor" d="M3 3.5 21 12 3 20.5l3.6-7.5L15 12 6.6 11l-3.6-7.5Z"/>
        </svg>
      </button>
    </div>
    {#if showToolbar}
      <ComposerToolbar
        showGifPicker={showGifPicker}
        onWrap={onApplyMarkdownWrap}
        onAppend={onAppendToComposer}
        onToggleGif={onToggleGif}
      />
    {/if}
  </div>
</form>
