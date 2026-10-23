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
if (!has(yaml, "tok-template", "{{ .hostname }}")) throw new Error("yaml template missing");

const commented = api.tokenize("# keep {{ .hostname }}\n", "yaml");
if (!has(commented, "tok-template", "{{ .hostname }}")) throw new Error("template inside a comment was not overlaid");
if (!commented.some((tok) => tok.cls === "tok-comment" && tok.text.includes("#"))) {
  throw new Error("comment text lost its class");
}

const quoted = api.tokenize('echo "{{ .hostname }}"\n', "shell");
if (!has(quoted, "tok-template", "{{ .hostname }}")) throw new Error("template inside a string was not overlaid");

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

console.log("editor tests passed");
