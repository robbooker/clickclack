export type MediaProbe = {
  width: number;
  height: number;
  durationMS: number;
};

export async function probeMediaDimensions(file: File): Promise<MediaProbe> {
  if (file.type.startsWith("image/")) {
    return probeImage(file);
  }
  if (file.type.startsWith("video/")) {
    return probeVideo(file);
  }
  return { width: 0, height: 0, durationMS: 0 };
}

function probeImage(file: File): Promise<MediaProbe> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      const out = { width: img.naturalWidth, height: img.naturalHeight, durationMS: 0 };
      URL.revokeObjectURL(url);
      resolve(out);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      resolve({ width: 0, height: 0, durationMS: 0 });
    };
    img.src = url;
  });
}

function probeVideo(file: File): Promise<MediaProbe> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    video.preload = "metadata";
    video.muted = true;
    const cleanup = () => {
      URL.revokeObjectURL(url);
      video.src = "";
    };
    video.onloadedmetadata = () => {
      const durationMS =
        Number.isFinite(video.duration) && video.duration > 0
          ? Math.round(video.duration * 1000)
          : 0;
      const out = { width: video.videoWidth, height: video.videoHeight, durationMS };
      cleanup();
      resolve(out);
    };
    video.onerror = () => {
      cleanup();
      resolve({ width: 0, height: 0, durationMS: 0 });
    };
    video.src = url;
  });
}
