package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

type testClient struct {
	handler http.Handler
	cookie  *http.Cookie
}

func newTestClient(t *testing.T, handler http.Handler) *testClient {
	t.Helper()
	return &testClient{handler: handler}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return newServer(db, false, os.DirFS("web")).handler()
}

func (c *testClient) request(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, input)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		request.AddCookie(c.cookie)
	}
	response := httptest.NewRecorder()
	c.handler.ServeHTTP(response, request)
	return response
}

func (c *testClient) register(t *testing.T, email, invite string) *httptest.ResponseRecorder {
	t.Helper()
	input := map[string]any{
		"email":        email,
		"password":     "correct horse battery staple",
		"display_name": "Test Member",
		"household":    "The Test Home",
	}
	if invite != "" {
		input["invite_token"] = invite
	}
	response := c.request(t, http.MethodPost, "/api/register", input)
	if response.Code == http.StatusOK {
		cookies := response.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("expected session cookie, got %d cookies", len(cookies))
		}
		c.cookie = cookies[0]
	}
	return response
}

func decodeResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, response.Body.String())
	}
	return value
}

func TestHouseholdInvitationSharesCookingData(t *testing.T) {
	handler := newTestHandler(t)
	owner := newTestClient(t, handler)
	if response := owner.register(t, "owner@example.test", ""); response.Code != http.StatusOK {
		t.Fatalf("register owner: status %d, body %s", response.Code, response.Body.String())
	}
	ownerAccount := decodeResponse[account](t, owner.request(t, http.MethodGet, "/api/me", nil))

	inviteResponse := owner.request(t, http.MethodPost, "/api/invites", map[string]any{})
	if inviteResponse.Code != http.StatusCreated {
		t.Fatalf("create invite: status %d, body %s", inviteResponse.Code, inviteResponse.Body.String())
	}
	invitation := decodeResponse[struct {
		Token string `json:"token"`
	}](t, inviteResponse)
	if invitation.Token == "" {
		t.Fatal("expected a non-empty invitation token")
	}

	member := newTestClient(t, handler)
	if response := member.register(t, "member@example.test", invitation.Token); response.Code != http.StatusOK {
		t.Fatalf("register invited member: status %d, body %s", response.Code, response.Body.String())
	}
	memberAccount := decodeResponse[account](t, member.request(t, http.MethodGet, "/api/me", nil))
	if memberAccount.HouseholdID != ownerAccount.HouseholdID {
		t.Fatalf("expected shared household %q, got %q", ownerAccount.HouseholdID, memberAccount.HouseholdID)
	}

	recipeResponse := owner.request(t, http.MethodPost, "/api/recipes", map[string]any{
		"name": "Roast vegetables", "servings": 2, "description": "Easy supper",
		"instructions": "Roast until tender.",
		"ingredients":  []map[string]string{{"name": "Carrot", "amount": "3", "unit": "whole"}},
	})
	if recipeResponse.Code != http.StatusCreated {
		t.Fatalf("create recipe: status %d, body %s", recipeResponse.Code, recipeResponse.Body.String())
	}
	recipeItem := decodeResponse[recipe](t, recipeResponse)
	sharedRecipes := decodeResponse[[]recipe](t, member.request(t, http.MethodGet, "/api/recipes", nil))
	if len(sharedRecipes) != 1 || sharedRecipes[0].ID != recipeItem.ID {
		t.Fatalf("expected invited member to see shared recipe, got %#v", sharedRecipes)
	}

	outside := newTestClient(t, handler)
	if response := outside.register(t, "outside@example.test", ""); response.Code != http.StatusOK {
		t.Fatalf("register outside user: status %d, body %s", response.Code, response.Body.String())
	}
	outsideRecipes := decodeResponse[[]recipe](t, outside.request(t, http.MethodGet, "/api/recipes", nil))
	if len(outsideRecipes) != 0 {
		t.Fatalf("outside household should not see recipes, got %#v", outsideRecipes)
	}
	if response := outside.request(t, http.MethodDelete, "/api/recipes/"+recipeItem.ID, nil); response.Code != http.StatusNotFound {
		t.Fatalf("outside household delete status = %d, want 404", response.Code)
	}
}

func TestMealPlanGeneratesAndEditsSharedShoppingList(t *testing.T) {
	handler := newTestHandler(t)
	client := newTestClient(t, handler)
	if response := client.register(t, "cook@example.test", ""); response.Code != http.StatusOK {
		t.Fatalf("register: status %d, body %s", response.Code, response.Body.String())
	}
	recipeResponse := client.request(t, http.MethodPost, "/api/recipes", map[string]any{
		"name": "Pancakes", "servings": 2, "ingredients": []map[string]string{
			{"name": "Flour", "amount": "2", "unit": "cups"},
			{"name": "Milk", "amount": "to taste", "unit": ""},
		},
	})
	if recipeResponse.Code != http.StatusCreated {
		t.Fatalf("create recipe: status %d, body %s", recipeResponse.Code, recipeResponse.Body.String())
	}
	recipe := decodeResponse[recipe](t, recipeResponse)
	weekStart := "2026-10-19"
	for _, servings := range []int{4, 2} {
		response := client.request(t, http.MethodPost, "/api/meals", map[string]any{
			"recipe_id": recipe.ID, "date": weekStart, "servings": servings,
		})
		if response.Code != http.StatusCreated {
			t.Fatalf("add meal: status %d, body %s", response.Code, response.Body.String())
		}
	}
	meals := decodeResponse[[]meal](t, client.request(t, http.MethodGet, "/api/meals?week_start="+weekStart, nil))
	if len(meals) != 2 {
		t.Fatalf("expected two planned meals, got %d", len(meals))
	}

	autoGenerated := decodeResponse[[]shoppingItem](t, client.request(t, http.MethodGet, "/api/shopping?week_start="+weekStart, nil))
	if len(autoGenerated) != 2 || autoGenerated[0].Amount != "6" {
		t.Fatalf("planned meals should keep the generated list in sync, got %#v", autoGenerated)
	}
	generated := client.request(t, http.MethodPost, "/api/shopping/generate", map[string]string{"week_start": weekStart})
	if generated.Code != http.StatusOK {
		t.Fatalf("generate shopping list: status %d, body %s", generated.Code, generated.Body.String())
	}
	items := decodeResponse[[]shoppingItem](t, generated)
	if len(items) != 2 {
		t.Fatalf("expected flour and milk in generated list, got %#v", items)
	}
	if items[0].Name != "Flour" || items[0].Amount != "6" || items[0].Unit != "cups" {
		t.Fatalf("flour should be combined and scaled to 6 cups, got %#v", items[0])
	}
	if items[1].Name != "Milk" || items[1].Amount != "to taste; to taste" {
		t.Fatalf("expected free-form amounts to be preserved, got %#v", items[1])
	}

	added := client.request(t, http.MethodPost, "/api/shopping", map[string]string{
		"name": "Tea", "amount": "1", "unit": "box", "week_start": weekStart,
	})
	if added.Code != http.StatusCreated {
		t.Fatalf("add manual shopping item: status %d, body %s", added.Code, added.Body.String())
	}
	items = decodeResponse[[]shoppingItem](t, client.request(t, http.MethodGet, "/api/shopping?week_start="+weekStart, nil))
	var tea shoppingItem
	for _, item := range items {
		if item.Name == "Tea" {
			tea = item
		}
	}
	if tea.ID == "" || tea.Unit != "box" || tea.Amount != "1" || tea.Source != "manual" {
		t.Fatalf("manual shopping item was not saved with amount and unit: %#v", tea)
	}
	checked := true
	patched := client.request(t, http.MethodPatch, "/api/shopping/"+tea.ID, map[string]any{"checked": checked})
	if patched.Code != http.StatusOK {
		t.Fatalf("check off shopping item: status %d, body %s", patched.Code, patched.Body.String())
	}
	if updated := decodeResponse[shoppingItem](t, patched); !updated.Checked {
		t.Fatal("expected checked item")
	}
	updatedRecipe := client.request(t, http.MethodPut, "/api/recipes/"+recipe.ID, map[string]any{
		"name": "Pancakes", "servings": 2,
		"ingredients": []map[string]string{
			{"name": "Flour", "amount": "3", "unit": "cups"},
			{"name": "Milk", "amount": "to taste", "unit": ""},
		},
	})
	if updatedRecipe.Code != http.StatusOK {
		t.Fatalf("update recipe: status %d, body %s", updatedRecipe.Code, updatedRecipe.Body.String())
	}
	items = decodeResponse[[]shoppingItem](t, client.request(t, http.MethodGet, "/api/shopping?week_start="+weekStart, nil))
	foundFlour, foundTea := false, false
	for _, item := range items {
		switch item.Name {
		case "Flour":
			foundFlour = item.Amount == "9"
		case "Tea":
			foundTea = item.Checked && item.Unit == "box"
		}
	}
	if !foundFlour || !foundTea {
		t.Fatalf("recipe edits should update generated amounts and preserve checked/manual items, got %#v", items)
	}
	if response := client.request(t, http.MethodDelete, "/api/meals/"+meals[0].ID, nil); response.Code != http.StatusNoContent {
		t.Fatalf("remove planned meal: status %d, body %s", response.Code, response.Body.String())
	}
	items = decodeResponse[[]shoppingItem](t, client.request(t, http.MethodGet, "/api/shopping?week_start="+weekStart, nil))
	for _, item := range items {
		if item.Name == "Flour" && item.Amount != "3" {
			t.Fatalf("removing a planned meal should update flour to 3 cups, got %#v", item)
		}
	}
	if response := client.request(t, http.MethodDelete, "/api/shopping/"+tea.ID, nil); response.Code != http.StatusNoContent {
		t.Fatalf("delete shopping item status = %d, want 204", response.Code)
	}
}

func TestAuthenticationValidationAndSession(t *testing.T) {
	handler := newTestHandler(t)
	anonymous := newTestClient(t, handler)
	if response := anonymous.request(t, http.MethodGet, "/", nil); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("Kompagnon — everyday, together")) {
		t.Fatalf("PWA shell response: status %d, body %s", response.Code, response.Body.String())
	}
	if response := anonymous.request(t, http.MethodGet, "/api/recipes", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated recipes status = %d, want 401", response.Code)
	}
	if response := anonymous.request(t, http.MethodPost, "/api/register", map[string]any{
		"email": "bad@example.test", "password": "short", "display_name": "Test", "household": "Test home",
	}); response.Code != http.StatusBadRequest {
		t.Fatalf("short password status = %d, want 400", response.Code)
	}
	client := newTestClient(t, handler)
	if response := client.register(t, "cook@example.test", ""); response.Code != http.StatusOK {
		t.Fatalf("register: status %d, body %s", response.Code, response.Body.String())
	}
	if response := client.register(t, "cook@example.test", ""); response.Code != http.StatusConflict {
		t.Fatalf("duplicate email status = %d, want 409", response.Code)
	}
	if response := client.request(t, http.MethodPost, "/api/logout", map[string]any{}); response.Code != http.StatusOK {
		t.Fatalf("logout: status %d, body %s", response.Code, response.Body.String())
	}
	if response := client.request(t, http.MethodGet, "/api/me", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("request after logout status = %d, want 401", response.Code)
	}
	if response := client.request(t, http.MethodPost, "/api/login", map[string]string{
		"email": "cook@example.test", "password": "incorrect password",
	}); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", response.Code)
	}
	login := client.request(t, http.MethodPost, "/api/login", map[string]string{
		"email": "cook@example.test", "password": "correct horse battery staple",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login: status %d, body %s", login.Code, login.Body.String())
	}
	client.cookie = login.Result().Cookies()[0]
	if response := client.request(t, http.MethodGet, "/api/me", nil); response.Code != http.StatusOK {
		t.Fatalf("request after login status = %d, want 200", response.Code)
	}
}
