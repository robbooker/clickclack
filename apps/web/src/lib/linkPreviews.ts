import { api } from "./api";
import type { LinkPreview } from "./types";

const previewRequests = new Map<string, Promise<LinkPreview | null>>();
const webURLPattern = /https?:\/\/[^\s<>"']+/gi;

function trimURLPunctuation(candidate: string): string {
  let value = candidate.replace(/[.,!?;:]+$/g, "");
  const pairs: Array<[string, string]> = [
    ["(", ")"],
    ["[", "]"],
    ["{", "}"],
  ];
  for (const [opening, closing] of pairs) {
    while (value.endsWith(closing)) {
      const openings = value.split(opening).length - 1;
      const closings = value.split(closing).length - 1;
      if (closings <= openings) break;
      value = value.slice(0, -1);
    }
  }
  return value;
}

export function firstPreviewURL(body: string): string | null {
  const prose = body.replace(/```[\s\S]*?```/g, " ").replace(/`[^`\n]*`/g, " ");
  for (const match of prose.matchAll(webURLPattern)) {
    const candidate = trimURLPunctuation(match[0]);
    if (candidate.length > 2048) continue;
    try {
      const parsed = new URL(candidate);
      if (parsed.protocol === "http:" || parsed.protocol === "https:") return parsed.toString();
    } catch {
      // Keep scanning in case a later URL is valid.
    }
  }
  return null;
}

export function loadLinkPreview(url: string): Promise<LinkPreview | null> {
  const cached = previewRequests.get(url);
  if (cached) return cached;
  const request = api<{ preview: LinkPreview }>(`/api/link-preview?url=${encodeURIComponent(url)}`)
    .then((result) => result?.preview ?? null)
    .catch(() => null);
  previewRequests.set(url, request);
  return request;
}
