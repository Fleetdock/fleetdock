// Shared by the root layout and lib/appearance.ts, so no "use client" here.

export const THEME_KEY = "theme";
export const STYLE_KEY = "ui-style";
export const BORDERS_KEY = "borders";
export const MOTION_KEY = "motion";

/**
 * APPEARANCE_SCRIPT sets .dark, data-style, data-borders and data-motion on <html> before first paint.
 * Keep it in step with apply() in lib/appearance.ts.
 */
export const APPEARANCE_SCRIPT = `(function(){var r=document.documentElement;try{var t=localStorage.getItem('${THEME_KEY}');var d=t==='dark'||(t!=='light'&&window.matchMedia('(prefers-color-scheme: dark)').matches);r.classList.toggle('dark',d);if(localStorage.getItem('${STYLE_KEY}')==='neutral')r.setAttribute('data-style','neutral');if(localStorage.getItem('${BORDERS_KEY}')==='none')r.setAttribute('data-borders','none');var m=localStorage.getItem('${MOTION_KEY}');if(m==='reduced'||m==='off')r.setAttribute('data-motion',m);}catch(e){}})();`;
