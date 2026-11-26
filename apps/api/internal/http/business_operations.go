package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
)

type operationRegister func(string, string, func(http.ResponseWriter, *http.Request, auth.Session))
type operationDecode func(http.ResponseWriter, *http.Request, any) bool
type operationFail func(http.ResponseWriter, *http.Request, error)

func operation[Input, Output any](register operationRegister, decode operationDecode, fail operationFail,
	path string, success int, run func(*http.Request, catalog.Tenant, Input) (Output, error)) {
	register("POST", "/api/v1/organizations/{id}/operations/"+path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		var in Input
		if !decode(w, r, &in) {
			return
		}
		out, err := run(r, catalog.Tenant{OrganizationID: r.PathValue("id"), Session: session}, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, success, out)
	})
}

func businessOperations(register operationRegister, decode operationDecode, fail operationFail, c catalog.Service, o orders.Service, svc servicebooking.Service) {
	operation(register, decode, fail, "catalog/search", 200, func(r *http.Request, t catalog.Tenant, in catalog.IndustrySearchFilters) (catalog.IndustrySearchResult, error) {
		return c.SearchIndustry(r.Context(), t, in)
	})
	operation(register, decode, fail, "catalog/fitment", 200, func(r *http.Request, t catalog.Tenant, in catalog.FitmentQuery) (catalog.FitmentResult, error) {
		return c.CheckFitment(r.Context(), t, in)
	})
	operation(register, decode, fail, "construction/calculate", 200, func(r *http.Request, t catalog.Tenant, in businessfoundation.ConstructionInput) (businessfoundation.ConstructionResult, error) {
		if err := o.CheckConstruction(r.Context(), t); err != nil {
			return businessfoundation.ConstructionResult{}, err
		}
		out, err := businessfoundation.CalculateConstruction(in)
		if err != nil {
			return out, auth.Failure(422, "INVALID_CALCULATION", "Проверьте размеры, единицы и параметры расчёта")
		}
		return out, nil
	})
	operation(register, decode, fail, "services/search", 200, func(r *http.Request, t catalog.Tenant, in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}) ([]servicebooking.Offering, error) {
		return svc.Search(r.Context(), t, in.Query, in.Limit)
	})
	operation(register, decode, fail, "services/availability", 200, func(r *http.Request, t catalog.Tenant, in servicebooking.Selection) (servicebooking.Availability, error) {
		return svc.Availability(r.Context(), t, in)
	})
	operation(register, decode, fail, "services/config/resources", 200, func(r *http.Request, t catalog.Tenant, in servicebooking.Resource) (servicebooking.Resource, error) {
		return svc.SaveResource(r.Context(), t, in)
	})
	operation(register, decode, fail, "services/config/offerings", 200, func(r *http.Request, t catalog.Tenant, in servicebooking.Offering) (servicebooking.Offering, error) {
		return svc.SaveOffering(r.Context(), t, in)
	})
	operation(register, decode, fail, "services/config/windows", 200, func(r *http.Request, t catalog.Tenant, in servicebooking.Window) (servicebooking.Window, error) {
		return svc.SaveWindow(r.Context(), t, in)
	})
	operation(register, decode, fail, "properties/search", 200, func(r *http.Request, t catalog.Tenant, in orders.PropertyFilter) (json.RawMessage, error) {
		return o.SearchProperties(r.Context(), t, in)
	})
	operation(register, decode, fail, "properties/discover", 200, func(r *http.Request, t catalog.Tenant, in orders.PropertySearch) (orders.PropertyPage, error) {
		return o.FindProperties(r.Context(), t, in)
	})
	register("GET", "/api/v1/organizations/{id}/operations/property-viewings/{viewing}", func(w http.ResponseWriter, r *http.Request, session auth.Session) {
		t := catalog.Tenant{OrganizationID: r.PathValue("id"), Session: session}
		out, err := o.GetPropertyViewing(r.Context(), t, r.PathValue("viewing"))
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, out)
	})
	operation(register, decode, fail, "services/work/assign", 200, func(r *http.Request, t catalog.Tenant, in orders.StaffAssignment) (json.RawMessage, error) {
		return o.AssignService(r.Context(), t, in)
	})
	operation(register, decode, fail, "services/work/receipt", 200, func(r *http.Request, t catalog.Tenant, in orders.ServiceReceipt) (json.RawMessage, error) {
		return o.RecordServiceReceipt(r.Context(), t, in)
	})
	for path, read := range map[string]func(*http.Request, catalog.Tenant) (json.RawMessage, error){
		"schedule": func(r *http.Request, t catalog.Tenant) (json.RawMessage, error) {
			return o.ServiceSchedule(r.Context(), t)
		},
		"financials": func(r *http.Request, t catalog.Tenant) (json.RawMessage, error) {
			return o.ServiceFinancials(r.Context(), t)
		},
		"orders/{order}": func(r *http.Request, t catalog.Tenant) (json.RawMessage, error) {
			return o.ServiceWorkState(r.Context(), t, r.PathValue("order"))
		},
	} {
		register("GET", "/api/v1/organizations/{id}/operations/services/work/"+path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			out, err := read(r, catalog.Tenant{OrganizationID: r.PathValue("id"), Session: session})
			if err != nil {
				fail(w, r, err)
				return
			}
			writeJSON(w, 200, out)
		})
	}
	operation(register, decode, fail, "requests", 201, func(r *http.Request, t catalog.Tenant, in orders.Intake) (json.RawMessage, error) {
		return o.PrepareIntake(r.Context(), t, in)
	})
}
