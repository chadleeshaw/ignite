/** @type {import('tailwindcss').Config} */
const igniteColors = {
  "primary": "#ff7400",
  "primary-content": "#1a0b06",
  "secondary": "#ffb401",
  "secondary-content": "#1a0b06",
  "accent": "#e4c8ff",
  "accent-content": "#121a3a",
  "neutral": "#c3cbe6",
  "neutral-content": "#121a3a",
  "base-100": "#121a3a",
  "base-200": "#0b1026",
  "base-300": "#1a1f45",
  "base-content": "#eef1ff",
  "info": "#a9d2ff",
  "info-content": "#121a3a",
  "success": "#6ee7b7",
  "success-content": "#1a0b06",
  "warning": "#ffc94d",
  "warning-content": "#1a0b06",
  "error": "#ffa48a",
  "error-content": "#1a0b06",
};

function igniteTheme(box, btn, badge) {
  return Object.assign({}, igniteColors, {
    "--rounded-box": box,
    "--rounded-btn": btn,
    "--rounded-badge": badge,
  });
}

module.exports = {
  content: [
    "./**/*.templ",
    "./**/*.html",
    "./**/*.go",
  ],
  plugins: [
    require("./public/node_modules/daisyui"),
    function ({ addVariant }) {
      addVariant("console", 'html[data-theme="console"] &');
    },
  ],
  daisyui: {
    themes: [
      {
        cozy: igniteTheme("1.125rem", "9999px", "9999px"),
        console: igniteTheme("0.625rem", "0.5rem", "0.375rem"),
      },
    ],
  },
};
