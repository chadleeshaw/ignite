// Shared preview highlighter. Tokens become elements with textContent.
// Nothing here writes HTML strings into the page.
(function (root) {
  "use strict";

  var LANGUAGES = {
    yaml: [
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" },
      { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
      { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
      { source: "^[ \\t]*[A-Za-z_][\\w.-]*(?=\\s*:)", flags: "ym", cls: "tok-key" },
      { source: "\\b(?:true|false|yes|no|null|on|off)\\b", flags: "y", cls: "tok-keyword" },
      { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" },
      { source: "^[ \\t]*-(?=\\s)", flags: "ym", cls: "tok-punct" }
    ],
    kickstart: [
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" },
      { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
      { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
      { source: "^%(?:packages|post|pre|end|addon|anaconda)\\b", flags: "ym", cls: "tok-section" },
      { source: "--[A-Za-z0-9-]+", flags: "y", cls: "tok-key" },
      { source: "\\b(?:auth|authconfig|autopart|bootloader|clearpart|cmdline|device|firewall|firstboot|graphical|group|halt|ignoredisk|install|keyboard|lang|logvol|network|part|poweroff|raid|reboot|repo|rootpw|selinux|services|shutdown|skipx|sshkey|text|timezone|upgrade|url|user|volgroup|xconfig|zerombr)\\b", flags: "y", cls: "tok-keyword" },
      { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" }
    ],
    preseed: [
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" },
      { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
      { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
      { source: "\\bd-i\\b", flags: "y", cls: "tok-keyword" },
      { source: "\\b(?:string|boolean|select|multiselect|note|password|error)\\b", flags: "y", cls: "tok-key" },
      { source: "\\b(?:true|false)\\b", flags: "y", cls: "tok-keyword" },
      { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" }
    ],
    ipxe: [
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" },
      { source: "\\$\\{[^}]+\\}", flags: "y", cls: "tok-var" },
      { source: "^#!ipxe\\b", flags: "ym", cls: "tok-keyword" },
      { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
      { source: "^:[A-Za-z_][\\w-]*", flags: "ym", cls: "tok-section" },
      { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
      { source: "\\b(?:boot|chain|choose|console|dhcp|echo|exit|goto|ifopen|imgfetch|imgfree|initrd|iseq|isset|item|kernel|menu|prompt|set|shell|show|sleep)\\b", flags: "y", cls: "tok-keyword" },
      { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" }
    ],
    shell: [
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" },
      { source: "\\$\\{[^}]+\\}|\\$[A-Za-z_]\\w*", flags: "y", cls: "tok-var" },
      { source: "^#![^\\n]*", flags: "ym", cls: "tok-keyword" },
      { source: "#[^\\n]*", flags: "y", cls: "tok-comment" },
      { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
      { source: "\\b(?:case|cd|do|done|echo|elif|else|esac|exit|export|fi|for|function|if|local|read|return|set|source|then|trap|while)\\b", flags: "y", cls: "tok-keyword" },
      { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" }
    ],
    ini: [
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" },
      { source: "#[^\\n]*|;[^\\n]*", flags: "y", cls: "tok-comment" },
      { source: "^[ \\t]*\\[[^\\]\\n]+\\]", flags: "ym", cls: "tok-section" },
      { source: "\"(?:\\\\.|[^\"\\\\\\n])*\"|'(?:\\\\.|[^'\\\\\\n])*'", flags: "y", cls: "tok-string" },
      { source: "\\b(?:APPEND|DEFAULT|DISPLAY|IMPLICIT|INCLUDE|KERNEL|LABEL|MENU|PROMPT|SAY|SERIAL|TIMEOUT|UI)\\b", flags: "yi", cls: "tok-keyword" },
      { source: "^[ \\t]*[A-Za-z_][\\w.-]*(?=\\s*=)", flags: "ym", cls: "tok-key" },
      { source: "(?<![\\w.])\\d+(?:\\.\\d+)?(?![\\w.])", flags: "y", cls: "tok-number" }
    ],
    xml: [
      { source: "<!--[\\s\\S]*?-->", flags: "y", cls: "tok-comment" },
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" },
      { source: "<!DOCTYPE\\b[^>]*>", flags: "yi", cls: "tok-keyword" },
      { source: "<\\/?[A-Za-z_][\\w:.-]*", flags: "y", cls: "tok-tag" },
      { source: "\\/?\\s*>", flags: "y", cls: "tok-punct" },
      { source: "[A-Za-z_:][\\w:.-]*(?=\\s*=)", flags: "y", cls: "tok-key" },
      { source: "\"(?:\\\\.|[^\"\\\\])*\"|'(?:\\\\.|[^'\\\\])*'", flags: "y", cls: "tok-string" }
    ],
    text: [
      { source: "\\{\\{[\\s\\S]*?\\}\\}", flags: "y", cls: "tok-var" }
    ]
  };

  function rulesFor(language) {
    var name = String(language || "text").toLowerCase();
    if (name === "yml") name = "yaml";
    if (name === "ks") name = "kickstart";
    if (name === "sh" || name === "bash") name = "shell";
    if (name === "pxe" || name === "cfg" || name === "conf" || name === "bootmenu") name = "ini";
    if (name === "yast" || name === "autoyast") name = "xml";
    return { name: LANGUAGES[name] ? name : "text", rules: LANGUAGES[name] || LANGUAGES.text };
  }

  function igniteTokenize(source, language) {
    var text = String(source == null ? "" : source);
    var compiled = rulesFor(language).rules.map(function (rule) {
      return { re: new RegExp(rule.source, rule.flags), cls: rule.cls };
    });
    var tokens = [];
    var i = 0;
    while (i < text.length) {
      var found = null;
      for (var r = 0; r < compiled.length; r++) {
        var re = compiled[r].re;
        re.lastIndex = i;
        var match = re.exec(text);
        if (match && match.index === i && match[0].length > 0) {
          found = { text: match[0], cls: compiled[r].cls };
          break;
        }
      }
      if (!found) {
        var prev = tokens[tokens.length - 1];
        if (prev && !prev.cls) prev.text += text.charAt(i);
        else tokens.push({ text: text.charAt(i), cls: "" });
        i++;
        continue;
      }
      tokens.push(found);
      i += found.text.length;
    }
    return tokens;
  }

  function igniteHighlightLanguage(filename, content) {
    var name = String(filename || "").toLowerCase();
    var base = name.split("/").pop();
    var dot = base.lastIndexOf(".");
    var ext = dot >= 0 ? base.slice(dot) : "";
    if (ext === ".yaml" || ext === ".yml" || name.indexOf("cloud-init") >= 0) return "yaml";
    if (ext === ".ks" || name.indexOf("kickstart") >= 0) return "kickstart";
    if (name.indexOf("preseed") >= 0) return "preseed";
    if (ext === ".ipxe" || name.indexOf("ipxe") >= 0) return "ipxe";
    if (ext === ".xml" || name.indexOf("autoyast") >= 0 || name.indexOf("yast") >= 0) return "xml";
    if (ext === ".sh" || ext === ".bash" || name.indexOf("shell") >= 0) return "shell";
    if (ext === ".cfg" || ext === ".conf" || name.indexOf("bootmenu") >= 0 || name.indexOf("pxelinux") >= 0) return "ini";
    var head = String(content || "").slice(0, 500);
    if (/^#!ipxe/m.test(head)) return "ipxe";
    if (/^#!/.test(head)) return "shell";
    if (/^#cloud-config/m.test(head)) return "yaml";
    if (/^#version=RHEL/m.test(head)) return "kickstart";
    if (/^d-i\s/m.test(head)) return "preseed";
    if (/^\s*</.test(head) && /<\/?[A-Za-z]/.test(head)) return "xml";
    return "text";
  }

  function igniteHighlight(container, source, language) {
    if (!container) return "";
    var text = String(source == null ? "" : source);
    var lang = rulesFor(language).name;
    var tokens = igniteTokenize(text, lang);
    container.replaceChildren();
    for (var i = 0; i < tokens.length; i++) {
      var tok = tokens[i];
      if (!tok.cls) {
        container.appendChild(document.createTextNode(tok.text));
        continue;
      }
      var span = document.createElement("span");
      span.className = tok.cls;
      span.textContent = tok.text;
      container.appendChild(span);
    }
    container.setAttribute("data-lang", lang);
    return lang;
  }

  function igniteHighlightNode(pre) {
    if (!pre || pre.getAttribute("data-highlighted") === "1") return;
    var source = pre.textContent;
    var lang = pre.getAttribute("data-lang") || igniteHighlightLanguage(pre.getAttribute("data-filename") || "", source);
    igniteHighlight(pre, source, lang);
    pre.setAttribute("data-highlighted", "1");
    var box = pre.closest ? pre.closest(".modal-box") : null;
    var badge = box ? box.querySelector("[data-hl-lang]") : null;
    if (badge) badge.textContent = pre.getAttribute("data-lang") || lang;
  }

  function igniteHighlightWithin(root) {
    var scope = root && root.querySelectorAll ? root : document;
    var nodes = scope.querySelectorAll("pre.hl");
    for (var i = 0; i < nodes.length; i++) igniteHighlightNode(nodes[i]);
  }

  document.addEventListener("htmx:afterSwap", function (evt) {
    var target = evt.detail && evt.detail.target;
    igniteHighlightWithin(target || document);
  });

  root.igniteTokenize = igniteTokenize;
  root.igniteHighlight = igniteHighlight;
  root.igniteHighlightLanguage = igniteHighlightLanguage;
  root.igniteHighlightWithin = igniteHighlightWithin;
})(typeof globalThis !== "undefined" ? globalThis : this);
