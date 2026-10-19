import {
  addDays,
  escapeHTML,
  formatIngredients,
  isoDate,
  mondayFor,
  parseIngredients,
  weekDays,
  weekRangeLabel,
} from "./app-core.js";

const $ = (selector) => document.querySelector(selector);
const app = {
  account: null,
  recipes: [],
  meals: [],
  shopping: [],
  weekStart: mondayFor(),
  authMode: "login",
  inviteToken: new URLSearchParams(location.search).get("invite") || "",
};

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...options, headers, credentials: "same-origin" });
  if (response.status === 204) return null;
  const result = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(result.error || "Something went wrong. Please try again.");
    error.status = response.status;
    if (response.status === 401 && app.account) {
      app.account = null;
      showAuth();
    }
    throw error;
  }
  return result;
}

function showAuth() {
  $("#app-view").hidden = true;
  $("#auth-view").hidden = false;
  updateAuthForm();
}

function showApp() {
  $("#auth-view").hidden = true;
  $("#app-view").hidden = false;
  $("#household-name").textContent = app.account.household;
}

function updateAuthForm() {
  const registering = app.authMode === "register";
  $("#auth-title").textContent = registering ? "Start with your household." : "Welcome back.";
  $("#auth-intro").textContent = app.inviteToken
    ? "Create an account to join the household that invited you."
    : "Keep recipes, meal plans, and the shopping list together with your household.";
  $("#name-field").hidden = !registering;
  $("#household-field").hidden = !registering || Boolean(app.inviteToken);
  $("#auth-submit").textContent = registering ? "Create account" : "Sign in";
  $("#auth-switch").textContent = registering ? "Already have an account? Sign in" : "Create a household account";
  $("#auth-form [name=password]").autocomplete = registering ? "new-password" : "current-password";
  $("#invite-note").hidden = !app.inviteToken;
  $("#invite-note").textContent = app.inviteToken ? "You have a household invitation. Create your account to join." : "";
}

async function start() {
  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/service-worker.js").catch(() => {});
  }
  try {
    app.account = await request("/api/me");
  } catch (error) {
    showAuth();
    if (error.status !== 401) showError("#auth-error", error.message);
    return;
  }
  showApp();
  try {
    await refresh();
  } catch (error) {
    showError("#app-error", error.message);
  }
}

async function refresh() {
  clearError("#app-error");
  await loadRecipes();
  await Promise.all([loadMeals(), loadShopping()]);
}

async function loadRecipes() {
  app.recipes = await request("/api/recipes");
  renderRecipes();
  if (!$("#page-plan").hidden) renderMeals();
}

async function loadMeals() {
  app.meals = await request(`/api/meals?week_start=${isoDate(app.weekStart)}`);
  renderMeals();
}

async function loadShopping() {
  app.shopping = await request(`/api/shopping?week_start=${isoDate(app.weekStart)}`);
  renderShopping();
}

function setPage(page) {
  document.querySelectorAll(".tab-button").forEach((button) => {
    const active = button.dataset.page === page;
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-current", active ? "page" : "false");
  });
  document.querySelectorAll(".workspace").forEach((section) => {
    section.hidden = section.id !== `page-${page}`;
  });
  if (page === "plan") renderMeals();
  if (page === "shopping") renderShopping();
}

function renderRecipes() {
  const list = $("#recipes-list");
  if (!app.recipes.length) {
    list.innerHTML = '<p class="empty-state">Your cookbook is ready. Add a favourite to get started.</p>';
    return;
  }
  list.innerHTML = app.recipes.map((recipe) => {
    const ingredientList = recipe.ingredients.length
      ? `<ul class="ingredient-preview">${recipe.ingredients.map((ingredient) => `
          <li>${escapeHTML([ingredient.amount, ingredient.unit, ingredient.name].filter(Boolean).join(" "))}</li>`).join("")}</ul>`
      : '<p class="muted">No ingredients added yet.</p>';
    const instructions = recipe.instructions
      ? `<details class="recipe-instructions"><summary>Instructions</summary><p>${escapeHTML(recipe.instructions).replace(/\n/g, "<br>")}</p></details>`
      : "";
    return `
      <article class="recipe-card">
        <div class="recipe-card-heading">
          <div><h3>${escapeHTML(recipe.name)}</h3><p class="muted">${escapeHTML(recipe.description || `Serves ${recipe.servings}`)}</p></div>
          <span class="serving-tag">Serves ${escapeHTML(recipe.servings)}</span>
        </div>
        ${ingredientList}
        ${instructions}
        <div class="card-actions">
          <button class="text-button" data-action="edit-recipe" data-id="${escapeHTML(recipe.id)}" type="button">Edit</button>
          <button class="text-button danger-text" data-action="delete-recipe" data-id="${escapeHTML(recipe.id)}" type="button">Delete</button>
        </div>
      </article>`;
  }).join("");
}

function renderMeals() {
  $("#week-label").textContent = weekRangeLabel(app.weekStart);
  const recipeOptions = app.recipes.map((recipe) =>
    `<option value="${escapeHTML(recipe.id)}">${escapeHTML(recipe.name)}</option>`).join("");
  $("#meal-days").innerHTML = weekDays(app.weekStart).map((date) => {
    const key = isoDate(date);
    const meals = app.meals.filter((meal) => meal.planned_date === key);
    const mealMarkup = meals.map((meal) => `
      <li class="meal-row">
        <span><strong>${escapeHTML(meal.recipe_name)}</strong><small>Serves ${escapeHTML(meal.servings)}</small></span>
        <button class="text-button danger-text" data-action="remove-meal" data-id="${escapeHTML(meal.id)}" type="button" aria-label="Remove ${escapeHTML(meal.recipe_name)}">Remove</button>
      </li>`).join("");
    const disabled = app.recipes.length ? "" : "disabled";
    return `
      <article class="day-card">
        <div class="day-heading">
          <div><h3>${new Intl.DateTimeFormat(undefined, { weekday: "long" }).format(date)}</h3><span class="muted">${new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(date)}</span></div>
        </div>
        ${meals.length ? `<ul class="meal-list">${mealMarkup}</ul>` : '<p class="muted day-empty">Nothing planned yet.</p>'}
        <form class="meal-add" data-date="${key}">
          <label class="sr-only">Recipe for ${key}
            <select name="recipe_id" required ${disabled}>
              <option value="">Choose a recipe</option>${recipeOptions}
            </select>
          </label>
          <label class="sr-only">Servings
            <input name="servings" type="number" min="1" max="100" value="4" required>
          </label>
          <button class="button button-secondary" type="submit" ${disabled}>Add meal</button>
        </form>
      </article>`;
  }).join("");
}

function renderShopping() {
  $("#shopping-week-label").textContent = `For ${weekRangeLabel(app.weekStart)}`;
  $("#shopping-empty").hidden = app.shopping.length > 0;
  $("#shopping-items").innerHTML = app.shopping.map((item) => `
    <li class="shopping-item ${item.checked ? "is-checked" : ""}" data-id="${escapeHTML(item.id)}">
      <label class="shopping-check">
        <input type="checkbox" data-action="check-item" ${item.checked ? "checked" : ""} aria-label="Mark ${escapeHTML(item.name)} as ${item.checked ? "not bought" : "bought"}">
        <span class="checkmark" aria-hidden="true"></span>
        <span class="shopping-name">${escapeHTML(item.name)}</span>
      </label>
      <span class="shopping-amount">${escapeHTML([item.amount, item.unit].filter(Boolean).join(" "))}</span>
      <details class="shopping-edit">
        <summary class="text-button">Edit</summary>
        <form class="shopping-edit-form">
          <label>Name<input name="name" value="${escapeHTML(item.name)}" required maxlength="120"></label>
          <label>Amount<input name="amount" value="${escapeHTML(item.amount)}" maxlength="40"></label>
          <label>Unit<input name="unit" value="${escapeHTML(item.unit)}" maxlength="40"></label>
          <button class="button button-secondary" type="submit">Save</button>
        </form>
      </details>
      <button class="icon-button remove-item" data-action="delete-item" type="button" aria-label="Remove ${escapeHTML(item.name)}">×</button>
    </li>`).join("");
}

function showError(selector, message) {
  const element = $(selector);
  element.textContent = message;
  element.hidden = false;
}

function clearError(selector) {
  const element = $(selector);
  element.textContent = "";
  element.hidden = true;
}

function recipeFormData(form) {
  const data = new FormData(form);
  return {
    name: data.get("name").trim(),
    servings: Number(data.get("servings")),
    description: data.get("description").trim(),
    instructions: data.get("instructions").trim(),
    ingredients: parseIngredients(data.get("ingredients")),
  };
}

function openRecipeForm(recipe = null) {
  const form = $("#recipe-form");
  form.hidden = false;
  form.elements.id.value = recipe?.id || "";
  form.elements.name.value = recipe?.name || "";
  form.elements.servings.value = recipe?.servings || 4;
  form.elements.description.value = recipe?.description || "";
  form.elements.instructions.value = recipe?.instructions || "";
  form.elements.ingredients.value = formatIngredients(recipe?.ingredients || []);
  $("#recipe-form-title").textContent = recipe ? "Edit recipe" : "Add a recipe";
  clearError("#recipe-error");
  form.elements.name.focus();
}

async function submitAuth(event) {
  event.preventDefault();
  clearError("#auth-error");
  const data = new FormData(event.currentTarget);
  const body = {
    email: data.get("email").trim(),
    password: data.get("password"),
  };
  if (app.authMode === "register") {
    body.display_name = data.get("display_name").trim();
    if (app.inviteToken) body.invite_token = app.inviteToken;
    else body.household = data.get("household").trim();
  }
  try {
    await request(`/api/${app.authMode}`, { method: "POST", body: JSON.stringify(body) });
    app.account = await request("/api/me");
    history.replaceState({}, "", "/");
    app.inviteToken = "";
    showApp();
    await refresh();
  } catch (error) {
    showError("#auth-error", error.message);
  }
}

$("#auth-form").addEventListener("submit", submitAuth);
$("#auth-switch").addEventListener("click", () => {
  app.authMode = app.authMode === "login" ? "register" : "login";
  updateAuthForm();
  clearError("#auth-error");
});
document.querySelectorAll(".tab-button").forEach((button) => {
  button.addEventListener("click", () => setPage(button.dataset.page));
});
$("#logout-button").addEventListener("click", async () => {
  await request("/api/logout", { method: "POST" }).catch(() => {});
  app.account = null;
  app.recipes = [];
  showAuth();
});
window.addEventListener("focus", () => {
  if (app.account) refresh().catch((error) => showError("#app-error", error.message));
});
$("#new-recipe-button").addEventListener("click", () => openRecipeForm());
$("#cancel-recipe").addEventListener("click", () => {
  $("#recipe-form").hidden = true;
  $("#recipe-form").reset();
});
$("#recipe-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  clearError("#recipe-error");
  const form = event.currentTarget;
  const id = form.elements.id.value;
  const path = id ? `/api/recipes/${encodeURIComponent(id)}` : "/api/recipes";
  try {
    await request(path, { method: id ? "PUT" : "POST", body: JSON.stringify(recipeFormData(form)) });
    form.hidden = true;
    form.reset();
    await Promise.all([loadRecipes(), loadMeals(), loadShopping()]);
  } catch (error) {
    showError("#recipe-error", error.message);
  }
});
$("#recipes-list").addEventListener("click", async (event) => {
  const button = event.target.closest("[data-action]");
  if (!button) return;
  const recipe = app.recipes.find((item) => item.id === button.dataset.id);
  if (button.dataset.action === "edit-recipe" && recipe) openRecipeForm(recipe);
  if (button.dataset.action === "delete-recipe" && recipe && confirm(`Delete "${recipe.name}"? Its planned meals will be removed too.`)) {
    try {
      await request(`/api/recipes/${encodeURIComponent(recipe.id)}`, { method: "DELETE" });
      await Promise.all([loadRecipes(), loadMeals(), loadShopping()]);
    } catch (error) {
      alert(error.message);
    }
  }
});
$("#meal-days").addEventListener("submit", async (event) => {
  if (!event.target.matches(".meal-add")) return;
  event.preventDefault();
  const data = new FormData(event.target);
  try {
    await request("/api/meals", {
      method: "POST",
      body: JSON.stringify({
        date: event.target.dataset.date,
        recipe_id: data.get("recipe_id"),
        servings: Number(data.get("servings")),
      }),
    });
    await Promise.all([loadMeals(), loadShopping()]);
  } catch (error) {
    alert(error.message);
  }
});
$("#meal-days").addEventListener("click", async (event) => {
  const button = event.target.closest('[data-action="remove-meal"]');
  if (!button) return;
  try {
    await request(`/api/meals/${encodeURIComponent(button.dataset.id)}`, { method: "DELETE" });
    await Promise.all([loadMeals(), loadShopping()]);
  } catch (error) {
    alert(error.message);
  }
});
$("#week-previous").addEventListener("click", async () => {
  app.weekStart = addDays(app.weekStart, -7);
  await Promise.all([loadMeals(), loadShopping()]);
});
$("#week-next").addEventListener("click", async () => {
  app.weekStart = addDays(app.weekStart, 7);
  await Promise.all([loadMeals(), loadShopping()]);
});
$("#shopping-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  clearError("#shopping-error");
  const form = event.currentTarget;
  const data = new FormData(form);
  try {
    await request("/api/shopping", {
      method: "POST",
      body: JSON.stringify({
        name: data.get("name").trim(),
        amount: data.get("amount").trim(),
        unit: data.get("unit").trim(),
        week_start: isoDate(app.weekStart),
      }),
    });
    form.reset();
    await loadShopping();
  } catch (error) {
    showError("#shopping-error", error.message);
  }
});
$("#shopping-items").addEventListener("change", async (event) => {
  if (!event.target.matches('[data-action="check-item"]')) return;
  const row = event.target.closest(".shopping-item");
  try {
    await request(`/api/shopping/${encodeURIComponent(row.dataset.id)}`, {
      method: "PATCH",
      body: JSON.stringify({ checked: event.target.checked }),
    });
    await loadShopping();
  } catch (error) {
    showError("#shopping-error", error.message);
  }
});
$("#shopping-items").addEventListener("submit", async (event) => {
  if (!event.target.matches(".shopping-edit-form")) return;
  event.preventDefault();
  const row = event.target.closest(".shopping-item");
  const data = new FormData(event.target);
  try {
    await request(`/api/shopping/${encodeURIComponent(row.dataset.id)}`, {
      method: "PATCH",
      body: JSON.stringify({
        name: data.get("name").trim(),
        amount: data.get("amount").trim(),
        unit: data.get("unit").trim(),
      }),
    });
    await loadShopping();
  } catch (error) {
    showError("#shopping-error", error.message);
  }
});
$("#shopping-items").addEventListener("click", async (event) => {
  const button = event.target.closest('[data-action="delete-item"]');
  if (!button) return;
  const row = button.closest(".shopping-item");
  try {
    await request(`/api/shopping/${encodeURIComponent(row.dataset.id)}`, { method: "DELETE" });
    await loadShopping();
  } catch (error) {
    showError("#shopping-error", error.message);
  }
});
$("#generate-list").addEventListener("click", async () => {
  clearError("#shopping-error");
  try {
    app.shopping = await request("/api/shopping/generate", {
      method: "POST",
      body: JSON.stringify({ week_start: isoDate(app.weekStart) }),
    });
    renderShopping();
  } catch (error) {
    showError("#shopping-error", error.message);
  }
});
$("#invite-button").addEventListener("click", async () => {
  try {
    const invitation = await request("/api/invites", { method: "POST" });
    $("#invite-link").textContent = `${location.origin}/?invite=${encodeURIComponent(invitation.token)}`;
    $("#invite-panel").hidden = false;
  } catch (error) {
    alert(error.message);
  }
});
$("#close-invite").addEventListener("click", () => { $("#invite-panel").hidden = true; });
$("#copy-invite").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText($("#invite-link").textContent);
    $("#copy-invite").textContent = "Copied";
  } catch {
    $("#invite-link").focus?.();
    alert("Copy this invitation link and send it to your household member.");
  }
});

start();
