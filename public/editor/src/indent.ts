const INDENT = "  ";

export interface Caret {
  value: string;
  start: number;
  end: number;
}

function outdentLine(line: string): { line: string; removed: number } {
  if (line.startsWith(INDENT)) return { line: line.slice(INDENT.length), removed: INDENT.length };
  if (line.startsWith("\t")) return { line: line.slice(1), removed: 1 };
  const spaces = /^ +/.exec(line);
  if (spaces) {
    const removed = Math.min(INDENT.length, spaces[0].length);
    return { line: line.slice(removed), removed };
  }
  return { line, removed: 0 };
}

// indentText inserts an indent at a collapsed caret. A selection, and Shift+Tab,
// changes every line the selection touches. Two spaces match the templates.
export function indentText(value: string, start: number, end: number, outdent: boolean): Caret {
  const anchor = Math.min(start, end);
  const focus = Math.max(start, end);
  if (!outdent && anchor === focus) {
    return {
      value: value.slice(0, anchor) + INDENT + value.slice(anchor),
      start: anchor + INDENT.length,
      end: anchor + INDENT.length,
    };
  }

  const lineStart = value.lastIndexOf("\n", Math.max(0, anchor - 1)) + 1;
  const lineBreak = value.indexOf("\n", focus);
  const blockEnd = lineBreak === -1 ? value.length : lineBreak;
  const lines = value.slice(lineStart, blockEnd).split("\n");
  let cursor = lineStart;
  let caret = anchor;
  const nextLines = lines.map((line) => {
    const origin = cursor;
    cursor += line.length + 1;
    if (!outdent) {
      if (origin <= anchor) caret += INDENT.length;
      return INDENT + line;
    }
    const out = outdentLine(line);
    if (origin <= anchor) caret -= Math.min(out.removed, Math.max(0, anchor - origin));
    return out.line;
  });
  const joined = nextLines.join("\n");
  const next = value.slice(0, lineStart) + joined + value.slice(blockEnd);
  if (anchor === focus) {
    const pos = Math.max(lineStart, Math.min(caret, lineStart + joined.length));
    return { value: next, start: pos, end: pos };
  }
  return { value: next, start: lineStart, end: lineStart + joined.length };
}
