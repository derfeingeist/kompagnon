import test from "node:test";
import assert from "node:assert/strict";
import {
  addDays,
  escapeHTML,
  formatIngredients,
  isoDate,
  mondayFor,
  parseIngredients,
  weekDays,
} from "./app-core.js";

test("mondayFor and weekDays build a Monday-through-Sunday week", () => {
  const monday = mondayFor(new Date(2026, 9, 21));
  assert.equal(isoDate(monday), "2026-10-19");
  assert.deepEqual(weekDays(monday).map(isoDate), [
    "2026-10-19", "2026-10-20", "2026-10-21", "2026-10-22",
    "2026-10-23", "2026-10-24", "2026-10-25",
  ]);
  assert.equal(isoDate(addDays(monday, 7)), "2026-10-26");
});

test("ingredient editor parses and formats one ingredient per line", () => {
  const ingredients = parseIngredients("Tomatoes | 800 | g\n Onion | 1 | whole \n\nOlive oil | 2 | tbsp");
  assert.deepEqual(ingredients, [
    { name: "Tomatoes", amount: "800", unit: "g" },
    { name: "Onion", amount: "1", unit: "whole" },
    { name: "Olive oil", amount: "2", unit: "tbsp" },
  ]);
  assert.equal(formatIngredients(ingredients), "Tomatoes | 800 | g\nOnion | 1 | whole\nOlive oil | 2 | tbsp");
});

test("recipe content is escaped before HTML rendering", () => {
  assert.equal(escapeHTML(`<script a="b">&'`), "&lt;script a=&quot;b&quot;&gt;&amp;&#39;");
});
