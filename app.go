package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "kompagnon_session"

type server struct {
	db           *sql.DB
	cookieSecure bool
	static       fs.FS
}

type recipeIngredient struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
	Unit   string `json:"unit"`
}

type recipe struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Instructions string             `json:"instructions"`
	Servings     int                `json:"servings"`
	Ingredients  []recipeIngredient `json:"ingredients"`
	UpdatedAt    int64              `json:"updated_at"`
}

type meal struct {
	ID          string `json:"id"`
	RecipeID    string `json:"recipe_id"`
	RecipeName  string `json:"recipe_name"`
	PlannedDate string `json:"planned_date"`
	Servings    int    `json:"servings"`
}

type shoppingItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Amount  string `json:"amount"`
	Unit    string `json:"unit"`
	Checked bool   `json:"checked"`
	Source  string `json:"source"`
}

func newServer(db *sql.DB, cookieSecure bool, static fs.FS) *server {
	return &server{db: db, cookieSecure: cookieSecure, static: static}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("POST /api/login", s.login)

	protected := func(pattern string, handler http.HandlerFunc) {
		mux.Handle(pattern, s.requireAuth(handler))
	}
	protected("POST /api/logout", s.logout)
	protected("GET /api/me", s.me)
	protected("POST /api/invites", s.createInvite)
	protected("GET /api/recipes", s.listRecipes)
	protected("POST /api/recipes", s.createRecipe)
	protected("GET /api/recipes/{id}", s.getRecipe)
	protected("PUT /api/recipes/{id}", s.updateRecipe)
	protected("DELETE /api/recipes/{id}", s.deleteRecipe)
	protected("GET /api/meals", s.listMeals)
	protected("POST /api/meals", s.createMeal)
	protected("DELETE /api/meals/{id}", s.deleteMeal)
	protected("GET /api/shopping", s.listShoppingItems)
	protected("POST /api/shopping", s.createShoppingItem)
	protected("POST /api/shopping/generate", s.generateShoppingItems)
	protected("PATCH /api/shopping/{id}", s.updateShoppingItem)
	protected("DELETE /api/shopping/{id}", s.deleteShoppingItem)

	mux.Handle("/", http.FileServer(http.FS(s.static)))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *server) listRecipes(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	rows, err := s.db.QueryContext(r.Context(), `
SELECT id,name,description,instructions,servings,updated_at
FROM recipes WHERE household_id=? ORDER BY name COLLATE NOCASE`, user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load recipes.")
		return
	}
	recipes := make([]recipe, 0)
	for rows.Next() {
		var item recipe
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Instructions, &item.Servings, &item.UpdatedAt); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "Could not load recipes.")
			return
		}
		recipes = append(recipes, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "Could not load recipes.")
		return
	}
	if err := rows.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load recipes.")
		return
	}
	for i := range recipes {
		ingredients, err := s.loadIngredients(r.Context(), recipes[i].ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not load recipes.")
			return
		}
		recipes[i].Ingredients = ingredients
	}
	writeJSON(w, http.StatusOK, recipes)
}

func (s *server) getRecipe(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	item, err := s.loadRecipe(r.Context(), user.HouseholdID, r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Recipe not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load recipe.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *server) createRecipe(w http.ResponseWriter, r *http.Request) {
	var input recipe
	if !decodeJSON(w, r, &input) {
		return
	}
	if message := validateRecipe(input); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	user := currentAccount(r)
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create recipe.")
		return
	}
	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create recipe.")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `
INSERT INTO recipes(id,household_id,name,description,instructions,servings,created_by,updated_at)
VALUES(?,?,?,?,?,?,?,?)`,
		id, user.HouseholdID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description),
		strings.TrimSpace(input.Instructions), input.Servings, user.ID, now); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create recipe.")
		return
	}
	if err := saveIngredients(r.Context(), tx, id, input.Ingredients); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create recipe.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create recipe.")
		return
	}
	created, err := s.loadRecipe(r.Context(), user.HouseholdID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Recipe was saved but could not be loaded.")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *server) updateRecipe(w http.ResponseWriter, r *http.Request) {
	var input recipe
	if !decodeJSON(w, r, &input) {
		return
	}
	if message := validateRecipe(input); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	user := currentAccount(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update recipe.")
		return
	}
	defer tx.Rollback()
	affectedWeeks, err := plannedWeeksForRecipe(r.Context(), tx, user.HouseholdID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update recipe.")
		return
	}
	result, err := tx.ExecContext(r.Context(), `
UPDATE recipes SET name=?,description=?,instructions=?,servings=?,updated_at=?
WHERE id=? AND household_id=?`, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description),
		strings.TrimSpace(input.Instructions), input.Servings, time.Now().UTC().Unix(), r.PathValue("id"), user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update recipe.")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update recipe.")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "Recipe not found.")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM recipe_ingredients WHERE recipe_id=?`, r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update recipe.")
		return
	}
	if err := saveIngredients(r.Context(), tx, r.PathValue("id"), input.Ingredients); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update recipe.")
		return
	}
	for _, weekStart := range affectedWeeks {
		if err := rebuildGeneratedShopping(r.Context(), tx, user.HouseholdID, weekStart, user.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not update recipe and its planned shopping lists.")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update recipe.")
		return
	}
	updated, err := s.loadRecipe(r.Context(), user.HouseholdID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Recipe was saved but could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not delete recipe.")
		return
	}
	defer tx.Rollback()
	affectedWeeks, err := plannedWeeksForRecipe(r.Context(), tx, user.HouseholdID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not delete recipe.")
		return
	}
	result, err := tx.ExecContext(r.Context(), `DELETE FROM recipes WHERE id=? AND household_id=?`,
		r.PathValue("id"), user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not delete recipe.")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not delete recipe.")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "Recipe not found.")
		return
	}
	for _, weekStart := range affectedWeeks {
		if err := rebuildGeneratedShopping(r.Context(), tx, user.HouseholdID, weekStart, user.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not delete recipe and update its planned shopping lists.")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not delete recipe.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) loadRecipe(ctx context.Context, householdID, id string) (recipe, error) {
	var item recipe
	err := s.db.QueryRowContext(ctx, `
SELECT id,name,description,instructions,servings,updated_at
FROM recipes WHERE id=? AND household_id=?`, id, householdID).
		Scan(&item.ID, &item.Name, &item.Description, &item.Instructions, &item.Servings, &item.UpdatedAt)
	if err != nil {
		return recipe{}, err
	}
	item.Ingredients, err = s.loadIngredients(ctx, id)
	return item, err
}

func (s *server) loadIngredients(ctx context.Context, recipeID string) ([]recipeIngredient, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name,amount,unit FROM recipe_ingredients WHERE recipe_id=? ORDER BY position`, recipeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]recipeIngredient, 0)
	for rows.Next() {
		var item recipeIngredient
		if err := rows.Scan(&item.Name, &item.Amount, &item.Unit); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func saveIngredients(ctx context.Context, tx *sql.Tx, recipeID string, ingredients []recipeIngredient) error {
	for i, item := range ingredients {
		id, err := newID()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO recipe_ingredients(id,recipe_id,name,amount,unit,position) VALUES(?,?,?,?,?,?)`,
			id, recipeID, strings.TrimSpace(item.Name), strings.TrimSpace(item.Amount), strings.TrimSpace(item.Unit), i); err != nil {
			return err
		}
	}
	return nil
}

func validateRecipe(item recipe) string {
	if len(strings.TrimSpace(item.Name)) < 1 || len(item.Name) > 120 {
		return "Recipe name must be between 1 and 120 characters."
	}
	if len(item.Description) > 1000 || len(item.Instructions) > 10000 {
		return "Recipe description or instructions are too long."
	}
	if item.Servings < 1 || item.Servings > 100 {
		return "Servings must be between 1 and 100."
	}
	if len(item.Ingredients) > 100 {
		return "A recipe can have at most 100 ingredients."
	}
	for _, ingredient := range item.Ingredients {
		if len(strings.TrimSpace(ingredient.Name)) < 1 || len(ingredient.Name) > 120 ||
			len(ingredient.Amount) > 40 || len(ingredient.Unit) > 40 {
			return "Each ingredient needs a name; ingredient details must be under 120 characters."
		}
	}
	return ""
}

func (s *server) listMeals(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	weekStart, ok := parseWeekStart(r.URL.Query().Get("week_start"))
	if !ok {
		writeError(w, http.StatusBadRequest, "week_start must be a Monday in YYYY-MM-DD format.")
		return
	}
	weekEnd := weekStart.AddDate(0, 0, 7).Format("2006-01-02")
	rows, err := s.db.QueryContext(r.Context(), `
SELECT m.id,m.recipe_id,r.name,m.planned_date,m.servings
FROM meal_plans m JOIN recipes r ON r.id=m.recipe_id
WHERE m.household_id=? AND m.planned_date>=? AND m.planned_date<?
ORDER BY m.planned_date,r.name COLLATE NOCASE`, user.HouseholdID, weekStart.Format("2006-01-02"), weekEnd)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load meal plan.")
		return
	}
	defer rows.Close()
	items := make([]meal, 0)
	for rows.Next() {
		var item meal
		if err := rows.Scan(&item.ID, &item.RecipeID, &item.RecipeName, &item.PlannedDate, &item.Servings); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not load meal plan.")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load meal plan.")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) createMeal(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RecipeID string `json:"recipe_id"`
		Date     string `json:"date"`
		Servings int    `json:"servings"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if _, err := time.Parse("2006-01-02", input.Date); err != nil {
		writeError(w, http.StatusBadRequest, "Date must use YYYY-MM-DD format.")
		return
	}
	if input.Servings < 1 || input.Servings > 100 {
		writeError(w, http.StatusBadRequest, "Servings must be between 1 and 100.")
		return
	}
	user := currentAccount(r)
	var recipeID string
	err := s.db.QueryRowContext(r.Context(), `SELECT id FROM recipes WHERE id=? AND household_id=?`,
		input.RecipeID, user.HouseholdID).Scan(&recipeID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Recipe not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add meal.")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add meal.")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add meal.")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO meal_plans(id,household_id,recipe_id,planned_date,servings,created_by,created_at) VALUES(?,?,?,?,?,?,?)`,
		id, user.HouseholdID, input.RecipeID, input.Date, input.Servings, user.ID, time.Now().UTC().Unix()); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add meal.")
		return
	}
	plannedDate, _ := time.Parse("2006-01-02", input.Date)
	if err := rebuildGeneratedShopping(r.Context(), tx, user.HouseholdID, isoWeekStart(plannedDate), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add meal and update its shopping list.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add meal.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *server) deleteMeal(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove meal.")
		return
	}
	defer tx.Rollback()
	var plannedDate string
	err = tx.QueryRowContext(r.Context(), `SELECT planned_date FROM meal_plans WHERE id=? AND household_id=?`,
		r.PathValue("id"), user.HouseholdID).Scan(&plannedDate)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Meal not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove meal.")
		return
	}
	result, err := tx.ExecContext(r.Context(), `DELETE FROM meal_plans WHERE id=? AND household_id=?`,
		r.PathValue("id"), user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove meal.")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove meal.")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "Meal not found.")
		return
	}
	date, err := time.Parse("2006-01-02", plannedDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove meal.")
		return
	}
	if err := rebuildGeneratedShopping(r.Context(), tx, user.HouseholdID, isoWeekStart(date), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove meal and update its shopping list.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove meal.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listShoppingItems(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	weekStart, ok := parseWeekStart(r.URL.Query().Get("week_start"))
	if !ok {
		writeError(w, http.StatusBadRequest, "week_start must be a Monday in YYYY-MM-DD format.")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT id,name,amount,unit,checked,source FROM shopping_items
WHERE household_id=? AND week_start=? ORDER BY checked,name COLLATE NOCASE,id`,
		user.HouseholdID, weekStart.Format("2006-01-02"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load shopping list.")
		return
	}
	defer rows.Close()
	items := make([]shoppingItem, 0)
	for rows.Next() {
		var item shoppingItem
		var checked int
		if err := rows.Scan(&item.ID, &item.Name, &item.Amount, &item.Unit, &checked, &item.Source); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not load shopping list.")
			return
		}
		item.Checked = checked != 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load shopping list.")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) createShoppingItem(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name      string `json:"name"`
		Amount    string `json:"amount"`
		Unit      string `json:"unit"`
		WeekStart string `json:"week_start"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 || len(input.Amount) > 40 || len(input.Unit) > 40 {
		writeError(w, http.StatusBadRequest, "Enter an item name under 120 characters and keep amount and unit under 40 characters.")
		return
	}
	weekStart, ok := parseWeekStart(input.WeekStart)
	if !ok {
		writeError(w, http.StatusBadRequest, "week_start must be a Monday in YYYY-MM-DD format.")
		return
	}
	user := currentAccount(r)
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add shopping item.")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `
INSERT INTO shopping_items(id,household_id,week_start,name,amount,unit,source,created_by,created_at)
VALUES(?,?,?,?,?,?,'manual',?,?)`, id, user.HouseholdID, weekStart.Format("2006-01-02"), input.Name,
		strings.TrimSpace(input.Amount), strings.TrimSpace(input.Unit), user.ID, time.Now().UTC().Unix())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not add shopping item.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *server) updateShoppingItem(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name    *string `json:"name"`
		Amount  *string `json:"amount"`
		Unit    *string `json:"unit"`
		Checked *bool   `json:"checked"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user := currentAccount(r)
	var item shoppingItem
	var checked int
	err := s.db.QueryRowContext(r.Context(), `
SELECT name,amount,unit,checked,source FROM shopping_items WHERE id=? AND household_id=?`,
		r.PathValue("id"), user.HouseholdID).Scan(&item.Name, &item.Amount, &item.Unit, &checked, &item.Source)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Shopping item not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update shopping item.")
		return
	}
	item.Checked = checked != 0
	if input.Name != nil {
		item.Name = strings.TrimSpace(*input.Name)
	}
	if input.Amount != nil {
		item.Amount = strings.TrimSpace(*input.Amount)
	}
	if input.Unit != nil {
		item.Unit = strings.TrimSpace(*input.Unit)
	}
	if input.Checked != nil {
		item.Checked = *input.Checked
	}
	if item.Name == "" || len(item.Name) > 120 || len(item.Amount) > 40 || len(item.Unit) > 40 {
		writeError(w, http.StatusBadRequest, "Item name is required and item fields must be within their length limits.")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `
UPDATE shopping_items SET name=?,amount=?,unit=?,checked=? WHERE id=? AND household_id=?`,
		item.Name, item.Amount, item.Unit, item.Checked, r.PathValue("id"), user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update shopping item.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *server) deleteShoppingItem(w http.ResponseWriter, r *http.Request) {
	user := currentAccount(r)
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM shopping_items WHERE id=? AND household_id=?`,
		r.PathValue("id"), user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove shopping item.")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove shopping item.")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "Shopping item not found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type shoppingGroup struct {
	name        string
	unit        string
	quantity    float64
	hasQuantity bool
	notes       []string
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *server) generateShoppingItems(w http.ResponseWriter, r *http.Request) {
	var input struct {
		WeekStart string `json:"week_start"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	weekStart, ok := parseWeekStart(input.WeekStart)
	if !ok {
		writeError(w, http.StatusBadRequest, "week_start must be a Monday in YYYY-MM-DD format.")
		return
	}
	user := currentAccount(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not prepare shopping list.")
		return
	}
	defer tx.Rollback()
	if err := rebuildGeneratedShopping(r.Context(), tx, user.HouseholdID, weekStart, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not prepare shopping list.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not prepare shopping list.")
		return
	}
	s.listShoppingItems(w, withQuery(r, "week_start", weekStart.Format("2006-01-02")))
}

func rebuildGeneratedShopping(ctx context.Context, executor sqlExecutor, householdID string, weekStart time.Time, userID string) error {
	start := weekStart.Format("2006-01-02")
	end := weekStart.AddDate(0, 0, 7).Format("2006-01-02")
	rows, err := executor.QueryContext(ctx, `
SELECT i.name,i.amount,i.unit,m.servings,r.servings
FROM meal_plans m
JOIN recipes r ON r.id=m.recipe_id
JOIN recipe_ingredients i ON i.recipe_id=r.id
WHERE m.household_id=? AND m.planned_date>=? AND m.planned_date<?
ORDER BY lower(i.name),lower(i.unit),m.planned_date,i.position`, householdID, start, end)
	if err != nil {
		return err
	}
	groups := make(map[string]*shoppingGroup)
	for rows.Next() {
		var name, amount, unit string
		var plannedServings, recipeServings int
		if err := rows.Scan(&name, &amount, &unit, &plannedServings, &recipeServings); err != nil {
			rows.Close()
			return err
		}
		key := shoppingGroupKey(name, unit)
		group := groups[key]
		if group == nil {
			group = &shoppingGroup{name: strings.TrimSpace(name), unit: strings.TrimSpace(unit)}
			groups[key] = group
		}
		if quantity, err := strconv.ParseFloat(strings.TrimSpace(amount), 64); err == nil && quantity >= 0 && !math.IsInf(quantity, 0) {
			group.quantity += quantity * float64(plannedServings) / float64(recipeServings)
			group.hasQuantity = true
		} else if strings.TrimSpace(amount) != "" {
			group.notes = append(group.notes, strings.TrimSpace(amount))
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	checkedItems := make(map[string]bool)
	rows, err = executor.QueryContext(ctx, `
SELECT name,unit,checked FROM shopping_items
WHERE household_id=? AND week_start=? AND source='generated'`, householdID, start)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name, unit string
		var checked int
		if err := rows.Scan(&name, &unit, &checked); err != nil {
			rows.Close()
			return err
		}
		if checked != 0 {
			checkedItems[shoppingGroupKey(name, unit)] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if _, err := executor.ExecContext(ctx, `DELETE FROM shopping_items WHERE household_id=? AND week_start=? AND source='generated'`,
		householdID, start); err != nil {
		return err
	}
	for _, key := range keys {
		group := groups[key]
		amount := ""
		if group.hasQuantity {
			amount = strconv.FormatFloat(group.quantity, 'f', -1, 64)
		}
		if len(group.notes) > 0 {
			if amount != "" {
				amount += "; "
			}
			amount += strings.Join(group.notes, "; ")
		}
		id, err := newID()
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `
INSERT INTO shopping_items(id,household_id,week_start,name,amount,unit,checked,source,created_by,created_at)
VALUES(?,?,?,?,?,?,?,'generated',?,?)`, id, householdID, start, group.name, amount, group.unit,
			checkedItems[key], userID, time.Now().UTC().Unix()); err != nil {
			return err
		}
	}
	return nil
}

func shoppingGroupKey(name, unit string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "\x00" + strings.ToLower(strings.TrimSpace(unit))
}

func plannedWeeksForRecipe(ctx context.Context, tx *sql.Tx, householdID, recipeID string) ([]time.Time, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT DISTINCT planned_date FROM meal_plans
WHERE household_id=? AND recipe_id=? ORDER BY planned_date`, householdID, recipeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	weeks := make(map[string]time.Time)
	for rows.Next() {
		var plannedDate string
		if err := rows.Scan(&plannedDate); err != nil {
			return nil, err
		}
		date, err := time.Parse("2006-01-02", plannedDate)
		if err != nil {
			return nil, err
		}
		week := isoWeekStart(date)
		weeks[week.Format("2006-01-02")] = week
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(weeks))
	for key := range weeks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]time.Time, 0, len(keys))
	for _, key := range keys {
		result = append(result, weeks[key])
	}
	return result, nil
}

func isoWeekStart(date time.Time) time.Time {
	utcDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	return utcDate.AddDate(0, 0, -((int(utcDate.Weekday()) + 6) % 7))
}

func withQuery(r *http.Request, key, value string) *http.Request {
	query := r.URL.Query()
	query.Set(key, value)
	url := *r.URL
	url.RawQuery = query.Encode()
	clone := r.Clone(r.Context())
	clone.URL = &url
	return clone
}

func parseWeekStart(value string) (time.Time, bool) {
	weekStart, err := time.Parse("2006-01-02", value)
	if err != nil || weekStart.Weekday() != time.Monday {
		return time.Time{}, false
	}
	return weekStart, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func describeAmount(amount, unit string) string {
	parts := make([]string, 0, 2)
	if amount != "" {
		parts = append(parts, amount)
	}
	if unit != "" {
		parts = append(parts, unit)
	}
	return strings.Join(parts, " ")
}
