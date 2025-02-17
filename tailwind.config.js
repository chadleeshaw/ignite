/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./**/*.html",
    "./**/*.templ",
    "./**/*.go",
  ],
  plugins: [
    require('./public/node_modules/daisyui'),
  ],
  daisyui: {
    themes: ["sunset"],
  },
}
