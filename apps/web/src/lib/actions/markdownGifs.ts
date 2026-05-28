const animatedURLKey = Symbol("clickclackAnimatedURL");

type EnhancedGIFImage = HTMLImageElement & {
	[animatedURLKey]?: string;
};

export function markdownImageViewerURL(image: HTMLImageElement) {
	return (image as EnhancedGIFImage)[animatedURLKey] || image.currentSrc || image.src;
}

export function enhanceMarkdownGifs(node: HTMLElement) {
	const timers = new Map<HTMLImageElement, number>();
	const plays = new Map<HTMLImageElement, number>();
	let destroyed = false;

	const gifStillURL = (src: string) => {
		try {
			const url = new URL(src, window.location.href);
			if (url.hostname !== "giphy.com" && !url.hostname.endsWith(".giphy.com")) return "";
			const giphy = url.pathname.match(/\/media\/(?:v1\.[^/]+\/)?([^/]+)\/giphy\.gif$/);
			if (giphy) return `${url.origin}/media/${giphy[1]}/giphy_s.gif`;
		} catch {
			return "";
		}
		return "";
	};

	const withReplayNonce = (src: string) => {
		const separator = src.includes("?") ? "&" : "?";
		return `${src}${separator}cc_replay=${Date.now()}`;
	};

	const playOnce = (
		wrapper: HTMLElement,
		image: HTMLImageElement,
		replay: HTMLButtonElement,
		animatedURL: string,
		stillURL: string,
		restart = false,
	) => {
		const previous = timers.get(image);
		if (previous) window.clearTimeout(previous);
		const token = (plays.get(image) || 0) + 1;
		plays.set(image, token);
		wrapper.classList.add("playing");
		wrapper.classList.remove("paused");
		replay.disabled = true;
		replay.tabIndex = -1;
		replay.ariaHidden = "true";
		let scheduled = false;
		const schedulePause = () => {
			if (scheduled) return;
			scheduled = true;
			if (destroyed || plays.get(image) !== token) return;
			const timer = window.setTimeout(() => {
				if (destroyed || plays.get(image) !== token) return;
				wrapper.classList.remove("playing");
				wrapper.classList.add("paused");
				replay.disabled = false;
				replay.tabIndex = 0;
				replay.ariaHidden = "false";
				if (stillURL) image.src = stillURL;
			}, 2600);
			timers.set(image, timer);
		};
		const waitForDecodedFrame = () => {
			void image
				.decode()
				.catch(() => {})
				.then(schedulePause);
		};
		image.addEventListener("load", waitForDecodedFrame, { once: true });
		if (restart) image.src = withReplayNonce(animatedURL);
		if (image.complete && image.naturalWidth > 0) waitForDecodedFrame();
	};

	const decorate = () => {
		for (const image of node.querySelectorAll<HTMLImageElement>("img")) {
			if (image.closest(".gif-player")) continue;
			if (image.closest("a")) continue;
			const animatedURL = image.getAttribute("src") || image.src;
			if (!/\.gif(?:$|[?#])/i.test(animatedURL)) continue;
			const stillURL = gifStillURL(animatedURL);
			if (!stillURL) continue;
			const wrapper = document.createElement("span");
			wrapper.className = "gif-player";
			const badge = document.createElement("span");
			badge.className = "gif-badge";
			badge.textContent = "GIF";
			const replay = document.createElement("button");
			replay.type = "button";
			replay.className = "gif-replay";
			replay.disabled = true;
			replay.tabIndex = -1;
			replay.ariaHidden = "true";
			replay.ariaLabel = `Replay GIF ${image.alt || "image"}`;
			replay.title = "Replay GIF";
			replay.textContent = "↻";
			(image as EnhancedGIFImage)[animatedURLKey] = animatedURL;
			image.parentNode?.insertBefore(wrapper, image);
			wrapper.append(image, badge, replay);
			replay.addEventListener("click", (event) => {
				event.preventDefault();
				event.stopPropagation();
				playOnce(wrapper, image, replay, animatedURL, stillURL, true);
			});
			playOnce(wrapper, image, replay, animatedURL, stillURL);
		}
	};

	const observer = new MutationObserver(decorate);
	observer.observe(node, { childList: true, subtree: true });
	decorate();

	return {
		destroy() {
			destroyed = true;
			observer.disconnect();
			for (const timer of timers.values()) window.clearTimeout(timer);
		},
	};
}
