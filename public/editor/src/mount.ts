import { detectLanguage, languageLabel, tokenize, type Token } from "./highlight";
import { indentText } from "./indent";

export function renderTokens(doc: Document, container: HTMLElement, tokens: Token[]): void {
  const frag = doc.createDocumentFragment();
  for (const token of tokens) {
    if (!token.cls) {
      frag.appendChild(doc.createTextNode(token.text));
      continue;
    }
    const span = doc.createElement("span");
    span.className = token.cls;
    span.textContent = token.text;
    frag.appendChild(span);
  }
  container.replaceChildren(frag);
}

function hookValue(textarea: HTMLTextAreaElement, onWrite: () => void): void {
  const proto = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value");
  if (!proto || !proto.get || !proto.set) return;
  Object.defineProperty(textarea, "value", {
    configurable: true,
    enumerable: true,
    get() {
      return proto.get!.call(this);
    },
    set(value: string) {
      proto.set!.call(this, value);
      onWrite();
    },
  });
}

// mountEditor paints a highlight layer behind the real textarea. The textarea
// stays the control that the form and the save path submit.
export function mountEditor(doc: Document | undefined): void {
  if (!doc) return;
  const textarea = doc.getElementById("editor-textarea");
  if (!(textarea instanceof HTMLTextAreaElement) || textarea.dataset.editorMounted === "1") return;
  textarea.dataset.editorMounted = "1";

  const field = textarea.parentElement;
  if (!field) return;
  let pre = field.querySelector<HTMLElement>(".editor-highlight");
  if (!pre) {
    pre = doc.createElement("pre");
    pre.className = "editor-highlight";
    pre.setAttribute("aria-hidden", "true");
    pre.appendChild(doc.createElement("code"));
    field.insertBefore(pre, textarea);
  }
  const code = pre.querySelector("code");
  if (!code) return;

  const host = doc.getElementById("code-edit");
  let releaseTab = false;
  let frame = 0;

  const syncScroll = () => {
    code.style.transform = `translate(${-textarea.scrollLeft}px, ${-textarea.scrollTop}px)`;
  };

  const paint = () => {
    const lang = detectLanguage(
      textarea.dataset.filename || "",
      textarea.value,
      textarea.dataset.language || "",
      textarea.dataset.category || "",
    );
    renderTokens(doc, code, tokenize(textarea.value, lang));
    const style = doc.defaultView?.getComputedStyle(textarea);
    if (style) {
      code.style.width = `${textarea.clientWidth}px`;
      code.style.padding = style.padding;
      code.style.font = style.font;
      code.style.lineHeight = style.lineHeight;
      code.style.letterSpacing = style.letterSpacing;
      code.style.tabSize = style.tabSize;
    }
    pre.dataset.lang = lang;
    const badge = doc.getElementById("editor-lang");
    if (badge) badge.textContent = textarea.value ? languageLabel(lang) : "";
    syncScroll();
  };

  const schedule = () => {
    const view = doc.defaultView;
    if (!view || frame) return;
    frame = view.requestAnimationFrame(() => {
      frame = 0;
      paint();
    });
  };

  hookValue(textarea, paint);
  textarea.addEventListener("input", schedule);
  textarea.addEventListener("ignite-editor-refresh", paint);
  textarea.addEventListener("scroll", syncScroll);
  textarea.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      releaseTab = true;
      return;
    }
    if (event.key !== "Tab") {
      if (event.key !== "Shift" && event.key !== "Control" && event.key !== "Alt" && event.key !== "Meta") {
        releaseTab = false;
      }
      return;
    }
    if (releaseTab || event.altKey || event.metaKey || event.ctrlKey) {
      releaseTab = false;
      return;
    }
    event.preventDefault();
    const next = indentText(textarea.value, textarea.selectionStart, textarea.selectionEnd, event.shiftKey);
    textarea.value = next.value;
    textarea.selectionStart = next.start;
    textarea.selectionEnd = next.end;
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });

  const view = doc.defaultView;
  view?.addEventListener("resize", schedule);
  const fontsReady = doc.fonts?.ready;
  if (fontsReady) fontsReady.then(() => paint()).catch(() => undefined);
  doc.querySelectorAll('link[rel="stylesheet"]').forEach((link) => {
    if ((link.getAttribute("href") || "").indexOf("editor.css") === -1) return;
    link.addEventListener("load", paint);
  });
  const theme = new MutationObserver(schedule);
  theme.observe(doc.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  host?.classList.add("hl-on");
  paint();
}
