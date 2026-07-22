import { useState } from "react";

export type Theme = "system" | "light" | "dark";
const storageKey = "flowlens-theme";

function initialTheme(): Theme {
  const stored = localStorage.getItem(storageKey);
  if (stored === "system" || stored === "light" || stored === "dark")
    return stored;
  if (stored !== null) localStorage.removeItem(storageKey);
  return "system";
}

export function ThemeSelect() {
  const [theme, setTheme] = useState<Theme>(initialTheme);

  function update(value: Theme) {
    setTheme(value);
    localStorage.setItem(storageKey, value);
    if (value === "system")
      document.documentElement.removeAttribute("data-theme");
    else document.documentElement.dataset.theme = value;
  }

  return (
    <div className="theme-toggle" aria-label="主题模式">
      <ThemeButton
        label="跟随系统"
        selected={theme === "system"}
        onClick={() => update("system")}
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="3.5" y="4.5" width="17" height="12" rx="2" />
          <path d="M8 20h8M12 16.5V20" />
        </svg>
      </ThemeButton>
      <ThemeButton
        label="浅色模式"
        selected={theme === "light"}
        onClick={() => update("light")}
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="3.5" />
          <path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M18.7 5.3l-1.4 1.4M6.7 17.3l-1.4 1.4" />
        </svg>
      </ThemeButton>
      <ThemeButton
        label="深色模式"
        selected={theme === "dark"}
        onClick={() => update("dark")}
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d="M20.2 15.4A8.5 8.5 0 0 1 8.6 3.8 8.5 8.5 0 1 0 20.2 15.4Z" />
        </svg>
      </ThemeButton>
    </div>
  );
}

function ThemeButton({
  label,
  selected,
  onClick,
  children,
}: {
  label: string;
  selected: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      className="theme-button"
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={selected}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
