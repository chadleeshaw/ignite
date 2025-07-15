/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./**/*.templ",
    "./**/*.html",
    "./**/*.go",
  ],
  plugins: [
    require('./public/node_modules/daisyui'),
  ],
  daisyui: {
    themes: ["sunset"],
  },
}