import * as esbuild from "esbuild";

await esbuild.build({
  entryPoints: ["src/editor.ts"],
  bundle: true,
  format: "iife",
  globalName: "igniteEditor",
  outfile: "../http/js/editor.js",
  minify: true,
  legalComments: "none",
  target: "es2019",
  charset: "utf8",
});

await esbuild.build({
  entryPoints: ["src/editor.css"],
  outfile: "../http/css/editor.css",
  minify: true,
  legalComments: "none",
});
