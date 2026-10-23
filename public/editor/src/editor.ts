import { detectLanguage, languageLabel, tokenize } from "./highlight";
import { indentText } from "./indent";
import { mountEditor, renderTokens } from "./mount";

export { detectLanguage, indentText, languageLabel, renderTokens, tokenize };

mountEditor(typeof document === "undefined" ? undefined : document);
