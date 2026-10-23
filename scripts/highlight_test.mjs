import { readFileSync } from "node:fs";
import { createContext, runInContext } from "node:vm";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const source = readFileSync(resolve(root, "public/http/js/highlight.js"), "utf8");
if (/\binnerHTML\b|\bouterHTML\b|insertAdjacentHTML/.test(source)) {
  throw new Error("highlighter writes HTML strings");
}

function makeDocument() {
  function node(name) {
    const el = {
      nodeName: name,
      className: "",
      attrs: {},
      children: [],
      set textContent(value) { el._text = String(value); },
      get textContent() {
        if (el.children.length === 0) return el._text || "";
        return el.children.map((child) => child.textContent || "").join("");
      },
      setAttribute(key, value) { el.attrs[key] = value; },
      getAttribute(key) { return el.attrs[key]; },
      appendChild(child) { el.children.push(child); return child; },
      replaceChildren() { el.children.length = 0; },
      querySelectorAll(sel) {
        const want = sel.replace("pre.", "");
        const out = [];
        const walk = (n) => {
          if (n.className && String(n.className).split(/\s+/).includes(want)) out.push(n);
          (n.children || []).forEach(walk);
        };
        walk(el);
        return out;
      },
    };
    el._text = "";
    return el;
  }
  return {
    createElement: (name) => node(String(name).toUpperCase()),
    createTextNode: (text) => ({ nodeName: "#text", textContent: String(text), children: [] }),
    addEventListener() {},
    querySelectorAll() { return []; },
  };
}

const sandbox = { console, document: makeDocument() };
sandbox.globalThis = sandbox;
createContext(sandbox);
runInContext(source, sandbox);

function joined(tokens) {
  return tokens.map((tok) => tok.text).join("");
}
function classes(tokens) {
  return new Set(tokens.map((tok) => tok.cls).filter(Boolean));
}

const samples = {
  yaml: "#cloud-config\nhostname: {{ .hostname }}\npackages:\n  - nginx\n",
  kickstart: "#version=RHEL8\ntext\nnetwork --bootproto=static --ip=10.0.0.8\n%packages\n",
  preseed: "# locale\nd-i netcfg/get_hostname string {{ .hostname }}\n",
  ipxe: "#!ipxe\necho Welcome {{ .hostname }}\nset net0/ip ${net0/ip}\n:start\n",
  shell: "#!/bin/sh\necho \"$HOME\"\nif [ -n \"$USER\" ]; then exit 0; fi\n",
  ini: "default linux\ntimeout 50\nLABEL linux\n  KERNEL vmlinuz\n",
  xml: "<!-- net -->\n<profile><ipaddr>{{ .ip }}</ipaddr></profile>\n",
};

for (const [lang, sample] of Object.entries(samples)) {
  const tokens = sandbox.igniteTokenize(sample, lang);
  if (joined(tokens) !== sample) {
    throw new Error(`${lang} dropped text\n${JSON.stringify(joined(tokens))}`);
  }
  const seen = classes(tokens);
  if (seen.size < 2) throw new Error(`${lang} produced too few token classes: ${[...seen]}`);
}

const attack = "<script>alert(1)</script>\n# not a tag\n";
for (const lang of ["yaml", "kickstart", "preseed", "ipxe", "shell", "ini", "xml", "text"]) {
  const pre = sandbox.document.createElement("pre");
  sandbox.igniteHighlight(pre, attack, lang);
  const scripts = pre.children.filter((child) => child.nodeName === "SCRIPT");
  if (scripts.length) throw new Error(`${lang} created a script element`);
  if (pre.textContent !== attack) throw new Error(`${lang} changed the text`);
  if (pre.children.some((child) => child.nodeName !== "SPAN" && child.nodeName !== "#text")) {
    throw new Error(`${lang} created an unexpected node`);
  }
}

if (sandbox.igniteHighlightLanguage("templates/cloud-init/default.templ", "") !== "yaml") {
  throw new Error("cloud-init path");
}
if (sandbox.igniteHighlightLanguage("boot.ipxe", "#!ipxe\n") !== "ipxe") {
  throw new Error("ipxe name");
}
if (sandbox.igniteHighlightLanguage("notes.txt", "#!/bin/bash\n") !== "shell") {
  throw new Error("shebang");
}

const yaml = sandbox.igniteTokenize("hostname: box\n", "yaml");
if (!yaml.some((tok) => tok.cls === "tok-key" && tok.text === "hostname")) {
  throw new Error("yaml key missing");
}
const ks = sandbox.igniteTokenize("network --hostname=box\n", "kickstart");
if (!ks.some((tok) => tok.cls === "tok-keyword" && tok.text === "network")) {
  throw new Error("kickstart keyword missing");
}
const pxe = sandbox.igniteTokenize("#!ipxe\necho hi\n", "ipxe");
if (!pxe.some((tok) => tok.cls === "tok-keyword" && tok.text === "#!ipxe")) {
  throw new Error("ipxe shebang missing");
}

console.log("highlight tests passed");
