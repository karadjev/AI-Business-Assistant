package httpapi

import (
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"net/http"
	"strconv"
	"strings"
)

func catalogRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	s := catalog.Service{Pool: identity.Pool}
	base := "/api/v1/organizations/{id}"
	routes := []struct{ method, path, operation string }{
		{"GET", "/categories", "categories"}, {"POST", "/categories", "category-create"}, {"PATCH", "/categories/{resource}", "category-update"},
		{"GET", "/products", "products"}, {"POST", "/products", "product-create"}, {"GET", "/products/{resource}", "product"}, {"PATCH", "/products/{resource}", "product-update"},
		{"POST", "/products/{resource}/publish", "publish"}, {"POST", "/products/{resource}/archive", "archive"},
		{"GET", "/products/{resource}/variants", "variants"}, {"POST", "/products/{resource}/variants", "variant-create"}, {"PATCH", "/variants/{resource}", "variant-update"},
		{"GET", "/inventory", "inventory"}, {"POST", "/inventory/adjustments", "adjust"}, {"GET", "/inventory/movements", "movements"},
	}
	for _, route := range routes {
		register(route.method, base+route.path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			t := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			id := r.PathValue("resource")
			var in catalog.Input
			if r.Method != "GET" {
				var raw map[string]json.RawMessage
				if !decode(w, r, &raw) {
					return
				}
				fields := map[string]string{
					"category-create": "name", "category-update": "name expected_version",
					"product-create": "name category_id description brand status sku variant_name attributes price_amount currency_code",
					"product-update": "name category_id description brand status expected_version",
					"publish":        "expected_version", "archive": "expected_version",
					"variant-create": "sku variant_name attributes price_amount currency_code", "variant-update": "sku variant_name attributes price_amount currency_code expected_version",
					"adjust": "variant_id quantity_on_hand expected_version reason",
				}[route.operation]
				for key, v := range raw {
					if !strings.Contains(" "+fields+" ", " "+key+" ") || string(v) == "null" && key != "category_id" {
						fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Неизвестное или пустое поле"))
						return
					}
				}
				required := map[string][]string{"product-create": {"name", "sku", "price_amount"}, "variant-create": {"sku", "price_amount"}, "variant-update": {"sku", "price_amount", "expected_version"}, "adjust": {"variant_id", "quantity_on_hand", "expected_version", "reason"}}[route.operation]
				for _, key := range required {
					if _, ok := raw[key]; !ok {
						fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Отсутствует обязательное поле"))
						return
					}
				}
				body, _ := json.Marshal(raw)
				if e := json.Unmarshal(body, &in); e != nil {
					fail(w, r, auth.Failure(400, "INVALID_JSON", "Некорректные типы полей"))
					return
				}

				if _, ok := raw["currency_code"]; ok && in.CurrencyCode == "" {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Валюта должна быть RUB"))
					return
				}
				if _, ok := raw["status"]; ok && in.Status == "" {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Статус не может быть пустым"))
					return
				}
			}
			// Endpoint-specific allowlist prevents accepting unrelated fields (including tenant IDs).
			q := r.URL.Query()
			f := catalog.Filters{Query: q.Get("q"), CategoryID: q.Get("category_id"), Status: q.Get("status"), Stock: q.Get("stock"), Sort: q.Get("sort")}
			for key, values := range q {
				if len(values) != 1 || (key != "q" && key != "category_id" && key != "status" && key != "stock" && key != "sort" && key != "page" && key != "page_size") {
					fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Неизвестный или повторный параметр"))
					return
				}
			}
			for key, target := range map[string]*int{"page": &f.Page, "page_size": &f.PageSize} {
				if q.Has(key) {
					n, e := strconv.Atoi(q.Get(key))
					if e != nil || n < 1 {
						fail(w, r, auth.Failure(422, "VALIDATION_ERROR", "Некорректная пагинация"))
						return
					}
					*target = n
				}
			}
			var result any
			var e error
			status := 200
			switch route.operation {
			case "categories":
				result, e = s.Categories(r.Context(), t)
			case "category-create", "category-update":
				result, e = s.Category(r.Context(), t, id, in)
				if route.operation == "category-create" {
					status = 201
				}
			case "products":
				result, e = s.Products(r.Context(), t, f, false)
			case "product-create":
				result, e = s.CreateProduct(r.Context(), t, in)
				status = 201
			case "product":
				result, e = s.Product(r.Context(), t, id, false)
			case "product-update":
				result, e = s.UpdateProduct(r.Context(), t, id, in, "")
			case "publish", "archive":
				result, e = s.UpdateProduct(r.Context(), t, id, in, route.operation)
			case "variants":
				result, e = s.Variants(r.Context(), t, id)
			case "variant-create":
				result, e = s.Variant(r.Context(), t, id, "", in)
				status = 201
			case "variant-update":
				result, e = s.Variant(r.Context(), t, "", id, in)
			case "inventory":
				result, e = s.Inventory(r.Context(), t, f, false)
			case "movements":
				result, e = s.Inventory(r.Context(), t, f, true)
			case "adjust":
				result, e = s.Adjust(r.Context(), t, in)
			}
			if e != nil {
				fail(w, r, e)
				return
			}
			writeJSON(w, status, result)
		})
	}
}
