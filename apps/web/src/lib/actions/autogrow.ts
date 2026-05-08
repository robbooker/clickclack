export function autoGrow(node: HTMLTextAreaElement, _value: string) {
  const resize = () => {
    const previous = node.style.height;
    node.style.height = "auto";
    const next = `${node.scrollHeight}px`;
    if (previous !== next) node.style.height = next;
    else node.style.height = previous;
  };
  const onInput = () => resize();
  const onWindowResize = () => resize();
  requestAnimationFrame(resize);
  node.addEventListener("input", onInput);
  window.addEventListener("resize", onWindowResize);
  return {
    update() {
      requestAnimationFrame(resize);
    },
    destroy() {
      node.removeEventListener("input", onInput);
      window.removeEventListener("resize", onWindowResize);
    },
  };
}
