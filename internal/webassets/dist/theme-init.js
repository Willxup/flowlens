const storageKey = "flowlens-theme";
const value = localStorage.getItem(storageKey);
if (value === "light" || value === "dark") {
  document.documentElement.dataset.theme = value;
} else {
  if (value !== null && value !== "system") localStorage.removeItem(storageKey);
  document.documentElement.removeAttribute("data-theme");
}
