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
    themes: [
      {
        ignite: {
          "primary": "#f97316",
          "secondary": "#FBBF24",
          "accent": "#FF4500",
          "neutral": "#1a1a1a",
          "base-100": "#2a2a2a",
          "info": "#3ABFF8",
          "success": "#36D399",
          "warning": "#FBBD23",
          "error": "#F87272",
        },
      },
      "sunset",
    ],
  },
}