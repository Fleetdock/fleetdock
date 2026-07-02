"use client";

import { useEffect, useState } from "react";

import { Moon, Sun } from "lucide-react";

export function ThemeToggle() {
  const [dark, setDark] = useState(false);

  useEffect(() => {
    setDark(document.documentElement.classList.contains("dark"));
  }, []);

  function toggle() {
    const el = document.documentElement;
    const next = !el.classList.contains("dark");
    el.classList.toggle("dark", next);
    window.localStorage.setItem("theme", next ? "dark" : "light");
    setDark(next);
  }

  return (
    <button className="btn btn-ghost btn-sm" onClick={toggle} aria-label="Toggle theme">
      {dark ? <Sun size={16} /> : <Moon size={16} />}
    </button>
  );
}
