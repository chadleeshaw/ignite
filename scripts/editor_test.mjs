import { readFileSync } from "node:fs";
import { createContext, runInContext } from "node:vm";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const bundle = readFileSync(resolve(root, "public/http/js/editor.js"), "utf8");
const source = [
  readFileSync(resolve(root, "public/editor/src/highlight.ts"), "utf8"),
  readFileSync(resolve(root, "public/editor/src/mount.ts"), "utf8"),
  readFileSync(resolve(root, "public/editor/src/editor.ts"), "utf8"),
  readFileSync(resolve(root, "templates/pages/provision.templ"), "utf8"),
].join("\n");

if (/\binnerHTML\b|\bouterHTML\b|insertAdjacentHTML|\bdocument\.write\b/.test(bundle)) {
  throw new Error("editor bundle writes HTML strings");
}
if (/\binnerHTML\b|\bouterHTML\b|insertAdjacentHTML/.test(source)) {
  throw new Error("editor source writes HTML strings");
}

const sandbox = { console };
sandbox.globalThis = sandbox;
createContext(sandbox);
runInContext(bundle, sandbox);
const api = sandbox.igniteEditor;
if (!api || typeof api.tokenize !== "function" || typeof api.detectLanguage !== "function") {
  throw new Error("editor bundle did not export igniteEditor");
}

function joined(tokens) {
  return tokens.map((tok) => tok.text).join("");
}
function classes(tokens) {
  return tokens.filter((tok) => tok.cls).map((tok) => tok.cls);
}
function has(tokens, cls, text) {
  return tokens.some((tok) => tok.cls === cls && (!text || tok.text === text));
}
function covers(tokens, cls, text) {
  return tokens.some((tok) => tok.cls === cls && tok.text.includes(text));
}

const samples = {
  yaml: "#cloud-config\nhostname: {{ .hostname }}\npackages:\n  - nginx\n",
  kickstart: "#version=RHEL8\ntext\nnetwork --hostname={{ .hostname }}\n%post\nif [ -n \"$USER\" ]; then echo ok; fi\n%end\n",
  preseed: "# locale\nd-i netcfg/get_hostname string {{ .hostname }}\n",
  ipxe: "#!ipxe\necho Welcome {{ .hostname }}\nset net0/ip ${net0/ip}\n:start\n",
  shell: "#!/bin/sh\necho \"$HOME\" {{ .hostname }}\nif [ -n \"$USER\" ]; then exit 0; fi\n",
  pxe: "default {{ .Name }}\ntimeout 50\nLABEL linux\n  KERNEL vmlinuz\n",
  xml: "<!-- net -->\n<profile><ipaddr>{{ .ip }}</ipaddr></profile>\n",
  text: "plain {{ .hostname }} text\n",
};

for (const [lang, sample] of Object.entries(samples)) {
  const tokens = api.tokenize(sample, lang);
  if (joined(tokens) !== sample) {
    throw new Error(`${lang} dropped text\n${JSON.stringify(joined(tokens))}`);
  }
  if (!has(tokens, "tok-template")) throw new Error(`${lang} missed the Go template overlay`);
  const seen = new Set(classes(tokens));
  if (lang !== "text" && seen.size < 2) throw new Error(`${lang} produced too few token classes: ${[...seen]}`);
}

const ks = api.tokenize(samples.kickstart, "kickstart");
if (!has(ks, "tok-keyword", "network")) throw new Error("kickstart keyword missing");
if (!has(ks, "tok-section", "%post")) throw new Error("kickstart %post missing");
if (!has(ks, "tok-keyword", "if")) throw new Error("kickstart %post did not use shell highlighting");
const beforePost = ks.slice(0, ks.findIndex((tok) => tok.text === "%post"));
if (beforePost.some((tok) => tok.cls === "tok-keyword" && tok.text === "if")) {
  throw new Error("shell keyword leaked into the kickstart command section");
}

const yaml = api.tokenize("hostname: {{ .hostname }}\n", "yaml");
if (!has(yaml, "tok-key", "hostname")) throw new Error("yaml key missing");
if (!covers(yaml, "tok-template", "{{") || !covers(yaml, "tok-template", "}}")) {
  throw new Error("yaml template delimiters missing");
}
if (!has(yaml, "tok-template-field", ".hostname")) throw new Error("yaml template field missing");

const plain = api.tokenize("# note\nhostname: jammy\npackage_update: true\n  - nginx\n", "yaml");
if (!has(plain, "tok-comment", "# note")) throw new Error("yaml comment missing");
if (!has(plain, "tok-key", "hostname")) throw new Error("yaml key missing on a plain value");
if (!has(plain, "tok-string", "jammy")) throw new Error("yaml plain string missing");
if (!has(plain, "tok-bool", "true")) throw new Error("yaml boolean missing");
if (!has(plain, "tok-string", "nginx")) throw new Error("yaml list string missing");
const nested = api.tokenize("  - content: |\n      network:\n", "yaml");
if (!has(nested, "tok-key", "content") || !has(nested, "tok-key", "network")) {
  throw new Error("yaml key inside a list was swallowed by the value");
}
if (nested.some((tok) => tok.cls === "tok-string" && tok.text.includes("content"))) {
  throw new Error("yaml key was classified as a string");
}

const commented = api.tokenize("# keep {{ .hostname }}\n", "yaml");
if (!covers(commented, "tok-template", "{{")) throw new Error("template inside a comment was not overlaid");
if (!has(commented, "tok-template-field", ".hostname")) throw new Error("template field inside a comment was not overlaid");
if (!commented.some((tok) => tok.cls === "tok-comment" && tok.text.includes("#"))) {
  throw new Error("comment text lost its class");
}

const quoted = api.tokenize('echo "{{ .hostname }}"\n', "shell");
if (!covers(quoted, "tok-template", "{{") || !has(quoted, "tok-template-field", ".hostname")) {
  throw new Error("template inside a string was not overlaid");
}

const attack = "<script>alert(1)</script>\n# {{ .hostname }}\n";
for (const lang of Object.keys(samples)) {
  const tokens = api.tokenize(attack, lang);
  if (joined(tokens) !== attack) throw new Error(`${lang} changed untrusted text`);
}

function node(name) {
  const el = {
    nodeName: name,
    className: "",
    children: [],
    set textContent(value) { el._text = String(value); },
    get textContent() {
      if (el.children.length === 0) return el._text || "";
      return el.children.map((child) => child.textContent || "").join("");
    },
    appendChild(child) { el.children.push(child); return child; },
    replaceChildren(...kids) {
      const flat = [];
      for (const kid of kids) {
        if (kid && kid.nodeName === "#fragment") flat.push(...kid.children);
        else flat.push(kid);
      }
      el.children = flat;
    },
  };
  el._text = "";
  return el;
}
const doc = {
  createElement: (name) => node(String(name).toUpperCase()),
  createTextNode: (text) => ({ nodeName: "#text", textContent: String(text), children: [] }),
  createDocumentFragment() {
    return {
      nodeName: "#fragment",
      children: [],
      appendChild(child) { this.children.push(child); return child; },
    };
  },
};
const pre = doc.createElement("pre");
api.renderTokens(doc, pre, api.tokenize(attack, "yaml"));
if (pre.children.some((child) => child.nodeName === "SCRIPT")) throw new Error("renderer created a script element");
if (pre.textContent !== attack) throw new Error("renderer changed the text");

const cases = [
  ["templates/cloud-init/default.templ", "", "yaml", "cloud-init", "yaml"],
  ["templates/kickstart/default.templ", "", "kickstart", "kickstart", "kickstart"],
  ["templates/preseed/default.templ", "", "text", "preseed", "preseed"],
  ["boot.ipxe", "#!ipxe\n", "", "", "ipxe"],
  ["setup.sh", "", "", "", "shell"],
  ["host.ks", "", "ini", "", "kickstart"],
  ["menu.cfg", "default linux\nLABEL local\n", "ini", "bootmenu", "pxe"],
  ["lab.cfg", "rootpw --iscrypted x\n%post\necho hi\n%end\n", "ini", "kickstart", "kickstart"],
  ["notes.txt", "#!/bin/bash\n", "", "", "shell"],
  ["auto.xml", "<profile></profile>", "", "autoyast", "xml"],
];
for (const [filename, content, hint, category, want] of cases) {
  const got = api.detectLanguage(filename, content, hint, category);
  if (got !== want) throw new Error(`detect ${filename} + ${category}: ${got} != ${want}`);
}

const inserted = api.indentText("hostname: box\n", 0, 0, false);
if (inserted.value !== "  hostname: box\n" || inserted.start !== 2) {
  throw new Error(`tab insert failed: ${JSON.stringify(inserted)}`);
}
const outdented = api.indentText("  hostname: box\n", 2, 2, true);
if (outdented.value !== "hostname: box\n" || outdented.start !== 0) {
  throw new Error(`tab outdent failed: ${JSON.stringify(outdented)}`);
}
const block = api.indentText("a\nb\n", 0, 3, false);
if (block.value !== "  a\n  b\n") throw new Error(`block indent failed: ${JSON.stringify(block)}`);

function channel(hex, i) {
  const v = parseInt(hex.slice(1 + i * 2, 3 + i * 2), 16) / 255;
  return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
}
function luminance(hex) {
  return 0.2126 * channel(hex, 0) + 0.7152 * channel(hex, 1) + 0.0722 * channel(hex, 2);
}
function contrast(a, b) {
  const hi = Math.max(luminance(a), luminance(b));
  const lo = Math.min(luminance(a), luminance(b));
  return (hi + 0.05) / (lo + 0.05);
}
function mix(bg, fg, alpha) {
  const parse = (hex) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  const [br, bgc, bb] = parse(bg);
  const [fr, fgf, fb] = parse(fg);
  const n = (c, d) => Math.round(alpha * c + (1 - alpha) * d);
  return `#${[n(fr, br), n(fgf, bgc), n(fb, bb)].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
}

const css = readFileSync(resolve(root, "public/editor/src/editor.css"), "utf8");
const colors = {};
for (const match of css.matchAll(/--hl-([a-z]+):\s*(#[0-9a-fA-F]{6})/g)) colors[match[1]] = match[2].toLowerCase();
const wash = css.match(/--hl-template-bg:\s*rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([0-9.]+)\s*\)/);
if (!wash) throw new Error("template background is not an rgba() wash");
const washHex = `#${[wash[1], wash[2], wash[3]].map((n) => Number(n).toString(16).padStart(2, "0")).join("")}`;
const washAlpha = Number(wash[4]);
const surfaces = ["#1a1f45", "#13172f", "#2c335c"];
const required = ["comment", "key", "string", "number", "keyword", "section", "template", "field", "var", "tag", "punct"];
for (const name of required) {
  if (!colors[name]) throw new Error(`missing --hl-${name}`);
  for (const surface of surfaces) {
    const ratio = contrast(colors[name], surface);
    if (ratio < 4.5) throw new Error(`--hl-${name} ${colors[name]} on ${surface} is ${ratio.toFixed(2)}:1`);
  }
}
for (const surface of surfaces) {
  const chip = mix(surface, washHex, washAlpha);
  for (const name of ["template", "field"]) {
    const ratio = contrast(colors[name], chip);
    if (ratio < 4.5) throw new Error(`--hl-${name} on the template chip (${chip}) is ${ratio.toFixed(2)}:1`);
  }
}
if (!/#code-edit \.tok-comment \{[^}]*font-style:\s*italic/.test(css)) {
  throw new Error("comments are not italic");
}

console.log("editor tests passed");
