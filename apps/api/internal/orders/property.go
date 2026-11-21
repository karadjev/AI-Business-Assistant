package orders

import (
	"context"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type PropertyType string
type TransactionType string
type PropertyStatus string
type PropertyAvailability string

type Property struct {
	ID                string               `json:"id"`
	OrganizationID    string               `json:"organization_id"`
	Title             string               `json:"title"`
	Type              PropertyType         `json:"property_type,omitempty"`
	Transaction       TransactionType      `json:"transaction_type,omitempty"`
	Description       string               `json:"description,omitempty"`
	Country           string               `json:"country,omitempty"`
	Region            string               `json:"region,omitempty"`
	City              string               `json:"city,omitempty"`
	District          string               `json:"district,omitempty"`
	Address           string               `json:"address,omitempty"`
	Locality          string               `json:"locality"`
	Price             *int64               `json:"price_amount,omitempty"`
	Currency          string               `json:"currency_code,omitempty"`
	Area              string               `json:"area_m2"`
	LivingArea        string               `json:"living_area_m2,omitempty"`
	LandArea          string               `json:"land_area_m2,omitempty"`
	Rooms             int                  `json:"rooms"`
	Bedrooms          *int                 `json:"bedrooms,omitempty"`
	Bathrooms         *int                 `json:"bathrooms,omitempty"`
	Floor             *int                 `json:"floor,omitempty"`
	TotalFloors       *int                 `json:"total_floors,omitempty"`
	BuildingType      string               `json:"building_type,omitempty"`
	YearBuilt         *int                 `json:"year_built,omitempty"`
	Renovation        string               `json:"renovation_status,omitempty"`
	Furnishing        string               `json:"furnishing_status,omitempty"`
	Elevator          *bool                `json:"has_elevator,omitempty"`
	Parking           *bool                `json:"has_parking,omitempty"`
	Status            PropertyStatus       `json:"property_status"`
	Availability      PropertyAvailability `json:"status"`
	PublishedAt       *time.Time           `json:"published_at,omitempty"`
	UpdatedAt         *time.Time           `json:"updated_at,omitempty"`
	VerifiedUntil     time.Time            `json:"verified_until"`
	ViewingOfferingID string               `json:"viewing_offering_id,omitempty"`
}

type PropertyCriteria struct {
	City         string          `json:"city,omitempty"`
	District     string          `json:"district,omitempty"`
	Type         PropertyType    `json:"property_type,omitempty"`
	Transaction  TransactionType `json:"transaction_type,omitempty"`
	Currency     string          `json:"currency_code,omitempty"`
	MinPrice     *int64          `json:"min_price_amount,omitempty"`
	MaxPrice     *int64          `json:"max_price_amount,omitempty"`
	MinArea      *float64        `json:"min_area_m2,omitempty"`
	MaxArea      *float64        `json:"max_area_m2,omitempty"`
	Rooms        *int            `json:"rooms,omitempty"`
	Bedrooms     *int            `json:"bedrooms,omitempty"`
	BuildingType string          `json:"building_type,omitempty"`
	Renovation   string          `json:"renovation_status,omitempty"`
	Floor        *int            `json:"floor,omitempty"`
}

type PropertySearch struct {
	PropertyCriteria
	Status       PropertyStatus       `json:"property_status,omitempty"`
	Availability PropertyAvailability `json:"availability,omitempty"`
	Sort         string               `json:"sort,omitempty"`
	Page         int                  `json:"page,omitempty"`
}

type PropertyPage struct {
	Items   []Property `json:"items"`
	Page    int        `json:"page"`
	HasMore bool       `json:"has_more"`
}

func oneOf[T ~string](value T, allowed ...T) bool {
	for _, v := range allowed {
		if value == v {
			return true
		}
	}
	return false
}

func bounded(n *int, min, max int) bool {
	return n == nil || (*n >= min && *n <= max)
}

func validArea(n *float64) bool {
	return n == nil || (!math.IsNaN(*n) && *n > 0 && *n <= 1000000)
}

func (f *PropertyCriteria) Normalize() {
	f.City = strings.TrimSpace(f.City)
	f.District = strings.TrimSpace(f.District)
	f.BuildingType = strings.TrimSpace(f.BuildingType)
	f.Currency = strings.ToUpper(strings.TrimSpace(f.Currency))
}

func (f PropertyCriteria) Valid() bool {
	for _, value := range []string{f.City, f.District, f.BuildingType} {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 120 || strings.ContainsFunc(value, unicode.IsControl) {
			return false
		}
	}
	return oneOf(f.Type, "", "apartment", "house", "commercial", "land", "new_development", "other") &&
		oneOf(f.Transaction, "", "sale", "long_term_rental", "short_term_rental") &&
		oneOf(f.Renovation, "", "shell", "needs_renovation", "renovated") &&
		(f.Currency == "" || len(f.Currency) == 3 && strings.Trim(f.Currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") == "") &&
		(f.MinPrice == nil || *f.MinPrice >= 0) && (f.MaxPrice == nil || *f.MaxPrice >= 0) &&
		(f.MinPrice == nil || f.MaxPrice == nil || *f.MinPrice <= *f.MaxPrice) &&
		validArea(f.MinArea) && validArea(f.MaxArea) &&
		(f.MinArea == nil || f.MaxArea == nil || *f.MinArea <= *f.MaxArea) &&
		bounded(f.Rooms, 0, 50) && bounded(f.Bedrooms, 0, 50) && bounded(f.Floor, -20, 300)
}

func (s Service) FindProperties(ctx context.Context, t catalog.Tenant, f PropertySearch) (PropertyPage, error) {
	var page PropertyPage
	f.Normalize()
	if !f.Valid() || f.Page < 0 || f.Page > 100 ||
		!oneOf(f.Sort, "", "price_asc", "price_desc", "area_asc", "area_desc", "newest") ||
		!oneOf(f.Status, "", "draft", "active", "reserved", "sold", "rented", "unavailable", "archived") ||
		!oneOf(f.Availability, "", "available", "unavailable", "unknown") ||
		(f.Currency == "" && (f.MinPrice != nil || f.MaxPrice != nil || strings.HasPrefix(f.Sort, "price_"))) {
		return page, invalidIntake()
	}
	if t.ChannelCapability != "" || t.AgentLease != "" || t.AgentTaskID != "" {
		return page, tenancy.Failure(403, "FORBIDDEN", "Поиск объектов доступен сотруднику")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	if err = kindAllowed(ctx, tx, t.OrganizationID, "property_viewing_request"); err != nil {
		return page, err
	}
	page.Page = max(1, f.Page)
	err = tx.QueryRow(ctx, `SELECT platform.property_search_page($1,$2::jsonb,$3)`, t.OrganizationID, f, page.Page).Scan(&page)
	if err != nil {
		return page, tenancy.Error(err)
	}
	return page, nil
}
