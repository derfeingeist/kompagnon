export function mondayFor(date = new Date()) {
  const result = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const daysSinceMonday = (result.getDay() + 6) % 7;
  result.setDate(result.getDate() - daysSinceMonday);
  return result;
}

export function addDays(date, amount) {
  const result = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  result.setDate(result.getDate() + amount);
  return result;
}

export function isoDate(date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

export function weekDays(weekStart) {
  return Array.from({ length: 7 }, (_, index) => addDays(weekStart, index));
}

export function weekRangeLabel(weekStart) {
  const end = addDays(weekStart, 6);
  const format = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" });
  const year = end.getFullYear() === weekStart.getFullYear() ? "" : `, ${end.getFullYear()}`;
  return `${format.format(weekStart)} – ${format.format(end)}${year}`;
}

export function parseIngredients(text) {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [name, amount = "", unit = ""] = line.split("|").map((part) => part.trim());
      return { name, amount, unit };
    })
    .filter((ingredient) => ingredient.name);
}

export function formatIngredients(ingredients = []) {
  return ingredients
    .map(({ name, amount = "", unit = "" }) => [name, amount, unit].join(" | "))
    .join("\n");
}

export function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  })[character]);
}
