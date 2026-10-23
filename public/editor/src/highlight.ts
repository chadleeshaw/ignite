export interface Token {
  text: string;
  cls: string;
}

interface Rule {
  source: string;
  flags: string;
  cls: string;
}

interface Range {
  start: number;
  end: number;
}

const TEMPLATE = /\{\{-?[\s\S]*?-?\}\}/g;

const YAML: Rule[] = [
  { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
  { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
  { source: "^[ \\t]*[A-Za-z_][\\w.-]*(?=\\s*:)", flags: "ym", cls: "tok-key" },
  { source: "\\b(?:true|false|yes|no|null|on|off)\\b", flags: "y", cls: "tok-keyword" },
  { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" },
  { source: "^[ \\t]*-(?=\\s)", flags: "ym", cls: "tok-punct" },
];

const KICKSTART: Rule[] = [
  { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
  { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
  { source: "^%(?:packages|post|pre|end|onerror|addon|anaconda)\\b[^\\n]*", flags: "ym", cls: "tok-section" },
  { source: "--[A-Za-z0-9-]+", flags: "y", cls: "tok-key" },
  { source: "\\b(?:auth|authconfig|autopart|bootloader|clearpart|cmdline|device|firewall|firstboot|graphical|group|halt|ignoredisk|install|keyboard|lang|logvol|network|part|poweroff|raid|reboot|repo|rootpw|selinux|services|shutdown|skipx|sshkey|text|timezone|upgrade|url|user|volgroup|xconfig|zerombr)\\b", flags: "y", cls: "tok-keyword" },
  { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" },
];

const SHELL: Rule[] = [
  { source: "\\$\\{[^}]+\\}|\\$[A-Za-z_]\\w*", flags: "y", cls: "tok-var" },
  { source: "^#!.*", flags: "ym", cls: "tok-keyword" },
  { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
  { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
  { source: "\\b(?:case|cd|do|done|echo|elif|else|esac|exit|export|fi|for|function|if|local|read|return|set|source|then|trap|while)\\b", flags: "y", cls: "tok-keyword" },
  { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" },
];

const PRESEED: Rule[] = [
  { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
  { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
  { source: "\\bd-i\\b", flags: "y", cls: "tok-keyword" },
  { source: "\\b[a-z0-9]+(?:/[A-Za-z0-9_.-]+)+", flags: "y", cls: "tok-key" },
  { source: "\\b(?:string|boolean|select|multiselect|note|password|error)\\b", flags: "y", cls: "tok-key" },
  { source: "\\b(?:true|false)\\b", flags: "y", cls: "tok-keyword" },
  { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" },
];

const IPXE: Rule[] = [
  { source: "\\$\\{[^}]+\\}", flags: "y", cls: "tok-var" },
  { source: "^#!ipxe\\b", flags: "ym", cls: "tok-keyword" },
  { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
  { source: "^:[A-Za-z_][\\w-]*", flags: "ym", cls: "tok-section" },
  { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
  { source: "\\b(?:boot|chain|choose|console|dhcp|echo|exit|goto|ifopen|imgfetch|imgfree|initrd|iseq|isset|item|kernel|menu|prompt|set|shell|show|sleep)\\b", flags: "y", cls: "tok-keyword" },
  { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" },
];

const PXE: Rule[] = [
  { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
  { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
  { source: "^[ \\t]*\\[[^\\]\\n]+\\]", flags: "ym", cls: "tok-section" },
  { source: "\\b(?:APPEND|DEFAULT|DISPLAY|IMPLICIT|INCLUDE|KERNEL|LABEL|LOCALBOOT|MENU|PROMPT|SAY|SERIAL|TIMEOUT|UI)\\b", flags: "yi", cls: "tok-keyword" },
  { source: "^[ \\t]*[A-Za-z_][\\w.-]*(?=\\s*=)", flags: "ym", cls: "tok-key" },
  { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" },
];

const XML: Rule[] = [
  { source: "<!--[\\s\\S]*?-->", flags: "y", cls: "tok-comment" },
  { source: "<!DOCTYPE\\b[^>]*>", flags: "yi", cls: "tok-keyword" },
  { source: "</?[A-Za-z_][\\w:.-]*", flags: "y", cls: "tok-tag" },
  { source: "/?\\s*>", flags: "y", cls: "tok-punct" },
  { source: "[A-Za-z_:][\\w:.-]*(?=\\s*=)", flags: "y", cls: "tok-key" },
  { source: "\"(?:\\\\.|[^\"\\\\])*\"|'(?:\\\\.|[^'\\\\])*'", flags: "y", cls: "tok-string" },
];

const RULES: Record<string, Rule[]> = {
  yaml: YAML,
  kickstart: KICKSTART,
  shell: SHELL,
  preseed: PRESEED,
  ipxe: IPXE,
  pxe: PXE,
  xml: XML,
  text: [],
};

const LABELS: Record<string, string> = {
  yaml: "YAML",
  kickstart: "Kickstart",
  shell: "Shell",
  preseed: "Preseed",
  ipxe: "iPXE",
  pxe: "PXE",
  xml: "XML",
  text: "Text",
};

export function languageLabel(language: string): string {
  return LABELS[language] || LABELS.text;
}

function compiled(rules: Rule[]): { re: RegExp; cls: string }[] {
  return rules.map((rule) => ({ re: new RegExp(rule.source, rule.flags), cls: rule.cls }));
}

const COMPILED: Record<string, { re: RegExp; cls: string }[]> = {};
for (const name of Object.keys(RULES)) COMPILED[name] = compiled(RULES[name]);

function templateRanges(text: string): Range[] {
  const ranges: Range[] = [];
  TEMPLATE.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = TEMPLATE.exec(text))) {
    if (match[0].length === 0) break;
    ranges.push({ start: match.index, end: match.index + match[0].length });
  }
  return ranges;
}

function maskTemplates(text: string, ranges: Range[]): string {
  if (!ranges.length) return text;
  let masked = "";
  let cursor = 0;
  for (const range of ranges) {
    masked += text.slice(cursor, range.start);
    masked += " ".repeat(range.end - range.start);
    cursor = range.end;
  }
  return masked + text.slice(cursor);
}

function tokenizeRules(text: string, language: string): Token[] {
  const rules = COMPILED[language] || COMPILED.text;
  const tokens: Token[] = [];
  let i = 0;
  while (i < text.length) {
    let found: Token | null = null;
    for (const rule of rules) {
      rule.re.lastIndex = i;
      const match = rule.re.exec(text);
      if (match && match.index === i && match[0].length > 0) {
        found = { text: match[0], cls: rule.cls };
        break;
      }
    }
    if (!found) {
      const prev = tokens[tokens.length - 1];
      const ch = text.charAt(i);
      if (prev && !prev.cls) prev.text += ch;
      else tokens.push({ text: ch, cls: "" });
      i++;
      continue;
    }
    tokens.push(found);
    i += found.text.length;
  }
  return tokens;
}

interface Segment {
  language: string;
  text: string;
}

function kickstartSegments(text: string): Segment[] {
  const segments: Segment[] = [];
  let shell = false;
  let i = 0;
  while (i < text.length) {
    const nl = text.indexOf("\n", i);
    const end = nl === -1 ? text.length : nl + 1;
    const line = text.slice(i, end);
    const body = line.endsWith("\n") ? line.slice(0, -1) : line;
    let language = shell ? "shell" : "kickstart";
    if (/^%end\b/.test(body) || /^%(?:packages|addon|anaconda)\b/.test(body)) {
      language = "kickstart";
      shell = false;
    } else if (/^%(?:post|pre|onerror)\b/.test(body)) {
      language = "kickstart";
      shell = true;
    }
    const prev = segments[segments.length - 1];
    if (prev && prev.language === language) prev.text += line;
    else segments.push({ language, text: line });
    i = end;
  }
  return segments;
}

function tokenizeMasked(text: string, language: string): Token[] {
  if (language !== "kickstart") return tokenizeRules(text, language);
  const tokens: Token[] = [];
  for (const segment of kickstartSegments(text)) {
    tokens.push(...tokenizeRules(segment.text, segment.language));
  }
  return tokens;
}

function project(original: string, tokens: Token[], ranges: Range[]): Token[] {
  const out: Token[] = [];
  const push = (text: string, cls: string) => {
    if (!text) return;
    const prev = out[out.length - 1];
    if (prev && prev.cls === cls) prev.text += text;
    else out.push({ text, cls });
  };
  let i = 0;
  for (const token of tokens) {
    let local = 0;
    while (local < token.text.length) {
      const abs = i + local;
      const range = ranges.find((item) => abs >= item.start && abs < item.end);
      if (range) {
        push(original.slice(range.start, range.end), "tok-template");
        local += range.end - abs;
        continue;
      }
      let next = token.text.length;
      for (const item of ranges) {
        if (item.start > abs && item.start < i + token.text.length) {
          next = Math.min(next, item.start - i);
        }
      }
      push(original.slice(abs, i + next), token.cls);
      local = next;
    }
    i += token.text.length;
  }
  return out;
}

export function tokenize(source: string, language: string): Token[] {
  const text = String(source ?? "");
  const lang = RULES[language] ? language : "text";
  const ranges = templateRanges(text);
  const masked = maskTemplates(text, ranges);
  return project(text, tokenizeMasked(masked, lang), ranges);
}

function extOf(filename: string): string {
  const base = filename.split(/[\\/]/).pop() || "";
  const dot = base.lastIndexOf(".");
  return dot >= 0 ? base.slice(dot).toLowerCase() : "";
}

function hasKind(blob: string, kinds: string[]): boolean {
  return kinds.some((kind) => blob.includes(kind));
}

function looksLikeKickstart(content: string): boolean {
  return /^#version=RHEL/m.test(content) || /^%(?:packages|post|pre|end|onerror)\b/m.test(content) || /^(?:autopart|bootloader|clearpart|rootpw|part)\b/m.test(content);
}

function looksLikePxe(content: string): boolean {
  return /^(?:default|prompt|timeout|label|kernel|append|menu)\b/im.test(content);
}

// detectLanguage picks a mode from the template kind, the extension, an
// explicit hint from the file list, then the text itself.
export function detectLanguage(filename = "", content = "", hint = "", category = ""): string {
  const name = String(filename || "");
  const text = String(content || "");
  const blob = `${name} ${category}`.toLowerCase();
  const ext = extOf(name);
  const hinted = String(hint || "").toLowerCase();

  if (hasKind(blob, ["cloud-init"])) return "yaml";
  if (hasKind(blob, ["kickstart"])) return "kickstart";
  if (hasKind(blob, ["preseed"])) return "preseed";
  if (hasKind(blob, ["ipxe"])) return "ipxe";
  if (hasKind(blob, ["autoyast", "yast"])) return "xml";
  if (hasKind(blob, ["bootmenu", "pxelinux"])) return "pxe";

  if (ext === ".yaml" || ext === ".yml") return "yaml";
  if (ext === ".ks") return "kickstart";
  if (ext === ".ipxe") return "ipxe";
  if (ext === ".xml") return "xml";
  if (ext === ".sh" || ext === ".bash") return "shell";
  if (ext === ".cfg" || ext === ".conf") {
    if (looksLikePxe(text) && !looksLikeKickstart(text)) return "pxe";
    return "kickstart";
  }

  if (hinted === "yaml" || hinted === "yml") return "yaml";
  if (hinted === "kickstart" || hinted === "ks") return "kickstart";
  if (hinted === "preseed") return "preseed";
  if (hinted === "ipxe") return "ipxe";
  if (hinted === "shell" || hinted === "sh" || hinted === "bash") return "shell";
  if (hinted === "xml") return "xml";
  if (hinted === "pxe") return "pxe";

  const head = text.slice(0, 800);
  if (/^#!ipxe/m.test(head)) return "ipxe";
  if (/^#cloud-config/m.test(head)) return "yaml";
  if (looksLikeKickstart(head)) return "kickstart";
  if (/^d-i\s/m.test(head)) return "preseed";
  if (/^#!/.test(head)) return "shell";
  if (/^\s*</.test(head) && /<\/?[A-Za-z]/.test(head)) return "xml";
  if (looksLikePxe(head)) return "pxe";
  if (hinted === "ini" || hinted === "cfg" || hinted === "conf") return looksLikeKickstart(text) ? "kickstart" : "pxe";
  return "text";
}
