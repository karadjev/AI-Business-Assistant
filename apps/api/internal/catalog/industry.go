package catalog

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
)

type IndustrySearchFilters struct {
	Industry       string                `json:"industry"`
	Text           []IndustryTextFilter  `json:"text,omitempty"`
	Ranges         []IndustryRangeFilter `json:"ranges,omitempty"`
	Vehicle        *FitmentVehicle       `json:"vehicle,omitempty"`
	Availability   IndustryAvailability  `json:"availability,omitempty"`
	CategoryID     string                `json:"category_id,omitempty"`
	Limit          int                   `json:"limit,omitempty"`
	AfterVariantID string                `json:"after_variant_id,omitempty"`
}

type IndustryTextMatch string

const (
	IndustryExact   IndustryTextMatch = "exact"
	IndustryPartial IndustryTextMatch = "partial" // literal, case-insensitive prefix
)

type IndustryTextFilter struct {
	Field string            `json:"field"`
	Match IndustryTextMatch `json:"match"`
	Value string            `json:"value"`
}

type IndustryRangeFilter struct {
	Field string `json:"field"`
	Min   string `json:"min,omitempty"` // Inclusive, canonical decimal text. Empty means unbounded.
	Max   string `json:"max,omitempty"`
}

type IndustryAvailability string

const (
	IndustryInStock    IndustryAvailability = "in_stock"
	IndustryOutOfStock IndustryAvailability = "out_of_stock"
	IndustryUnknown    IndustryAvailability = "unknown"
)

type IndustrySearchResult struct {
	Items              []IndustrySearchItem `json:"items"`
	NextAfterVariantID string               `json:"next_after_variant_id,omitempty"`
}

type IndustrySearchItem struct {
	ProductID    string `json:"product_id"`
	VariantID    string `json:"variant_id"`
	Name         string `json:"name"`
	Brand        string `json:"brand"`
	SKU          string `json:"sku"`
	VariantName  string `json:"variant_name"`
	PriceAmount  int64  `json:"price_amount"`
	CurrencyCode string `json:"currency_code"`
	Freshness    string `json:"freshness"`
	Available    *int64 `json:"available"`
}

type industryField struct {
	expression string
	industries string // space-delimited canonical profile codes; empty means general
	integer    bool
}

var industryTextFields = map[string]industryField{
	"name":         {expression: `lower(p.name) COLLATE "C"`},
	"brand":        {expression: `lower(p.brand) COLLATE "C"`},
	"sku":          {expression: `lower(v.sku) COLLATE "C"`},
	"variant_name": {expression: `lower(v.variant_name) COLLATE "C"`},
	"oem":          {expression: industryTextAttribute("oem"), industries: "auto_parts"},
	"manufacturer": {expression: industryTextAttribute("manufacturer"), industries: "auto_parts"},
	"model":        {expression: industryTextAttribute("model"), industries: "electronics"},
	"color":        {expression: industryTextAttribute("color"), industries: "electronics apparel furniture mixed_nonfood"},
	"material":     {expression: industryTextAttribute("material"), industries: "apparel furniture building_materials mixed_nonfood"},
	"size":         {expression: industryTextAttribute("size"), industries: "apparel"},
	"size_system":  {expression: industryTextAttribute("size_system"), industries: "apparel"},
	"selling_unit": {expression: industryTextAttribute("selling_unit"), industries: "building_materials"},
}

var industryRangeFields = map[string]industryField{
	"price_amount":    {expression: "v.price_amount", integer: true},
	"memory_gb":       {expression: industryNumericAttribute("memory_gb", true), industries: "electronics", integer: true},
	"warranty_months": {expression: industryNumericAttribute("warranty_months", true), industries: "electronics", integer: true},
	"weight_kg":       {expression: industryNumericAttribute("weight_kg", false), industries: "building_materials furniture mixed_nonfood"},
	"volume_l":        {expression: industryNumericAttribute("volume_l", false), industries: "building_materials mixed_nonfood"},
	"length_mm":       {expression: industryNumericAttribute("length_mm", false), industries: "building_materials furniture mixed_nonfood"},
	"width_mm":        {expression: industryNumericAttribute("width_mm", false), industries: "building_materials furniture mixed_nonfood"},
	"height_mm":       {expression: industryNumericAttribute("height_mm", false), industries: "building_materials furniture mixed_nonfood"},
	"package_quantity": {expression: industryNumericAttribute("package_quantity", true),
		industries: "building_materials mixed_nonfood", integer: true},
}

func industryTextAttribute(key string) string {
	return `platform.industry_catalog_text(v.attributes, '` + key + `') COLLATE "C"`
}

func industryNumericAttribute(key string, integer bool) string {
	return fmt.Sprintf("platform.industry_catalog_number(v.attributes, '%s', %t)", key, integer)
}

func (f industryField) allowed(industry string) bool {
	return f.expression != "" && (f.industries == "" || strings.Contains(" "+f.industries+" ", " "+industry+" "))
}

var industryDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,9})?$`)
var industryLikeEscape = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func industryBound(s string, integer bool) (*big.Rat, bool) {
	if !industryDecimal.MatchString(s) || (integer && strings.Contains(s, ".")) {
		return nil, false
	}
	return new(big.Rat).SetString(s)
}

func (f *IndustrySearchFilters) predicates(org string) ([]string, []any, error) {
	switch f.Industry {
	case "electronics", "auto_parts", "building_materials", "apparel", "furniture", "mixed_nonfood":
	default:
		return nil, nil, invalid("Отрасль не поддерживает поиск по каталогу")
	}
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 50 || len(f.Text) > 4 || len(f.Ranges) > 4 {
		return nil, nil, invalid("Превышен лимит поиска по каталогу")
	}
	where := []string{"v.organization_id=$1", "p.status='active'", "(SELECT platform.industry_catalog_profile($1))=$2"}
	args := []any{org, f.Industry}
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	if f.CategoryID != "" {
		if !organizations.ValidID(f.CategoryID) {
			return nil, nil, invalid("Некорректная категория")
		}
		add("p.category_id=$%d", f.CategoryID)
	}
	if f.AfterVariantID != "" {
		if !organizations.ValidID(f.AfterVariantID) {
			return nil, nil, invalid("Некорректный курсор")
		}
		add("v.id>$%d", f.AfterVariantID)
	}
	seen := make(map[string]bool)
	for _, filter := range f.Text {
		field := industryTextFields[filter.Field]
		value := strings.TrimSpace(filter.Value)
		if !field.allowed(f.Industry) || seen[filter.Field] || !textValid(value, 1, 100) {
			return nil, nil, invalid("Некорректный текстовый фильтр")
		}
		seen[filter.Field] = true
		switch filter.Match {
		case IndustryExact:
			add(field.expression+"=lower($%d::text)", value)
		case IndustryPartial:
			if !textValid(value, 2, 100) {
				return nil, nil, invalid("Слишком короткий префикс поиска")
			}
			add(field.expression+" LIKE lower($%d::text)", industryLikeEscape.Replace(value)+"%")
		default:
			return nil, nil, invalid("Некорректный режим поиска")
		}
	}
	for _, filter := range f.Ranges {
		field := industryRangeFields[filter.Field]
		if !field.allowed(f.Industry) || seen[filter.Field] || (filter.Min == "" && filter.Max == "") {
			return nil, nil, invalid("Некорректный диапазон поиска")
		}
		seen[filter.Field] = true
		boundType := "::text::numeric"
		if filter.Field == "price_amount" {
			boundType = "::text::bigint"
		}
		var lower *big.Rat
		if filter.Min != "" {
			value, ok := industryBound(filter.Min, field.integer)
			if !ok {
				return nil, nil, invalid("Некорректная нижняя граница")
			}
			lower = value
			add(field.expression+">=$%d"+boundType, filter.Min)
		}
		if filter.Max != "" {
			value, ok := industryBound(filter.Max, field.integer)
			if !ok || (lower != nil && value.Cmp(lower) < 0) {
				return nil, nil, invalid("Некорректная верхняя граница")
			}
			add(field.expression+"<=$%d"+boundType, filter.Max)
		}
	}
	if f.Vehicle != nil {
		if f.Industry != "auto_parts" {
			return nil, nil, invalid("Поиск по автомобилю доступен для автозапчастей")
		}
		vehicle, err := f.Vehicle.normalized()
		if err != nil {
			return nil, nil, err
		}
		first := len(args) + 1
		args = append(args, vehicle.Make, vehicle.Model, vehicle.Engine, vehicle.Year)
		match := fitmentEvidenceMatch(first)
		where = append(where,
			"EXISTS(SELECT 1 FROM platform.catalog_fitment_evidence f WHERE f.compatible AND "+match+")",
			"NOT EXISTS(SELECT 1 FROM platform.catalog_fitment_evidence f WHERE NOT f.compatible AND "+match+")")
	}
	switch f.Availability {
	case "":
	case IndustryInStock:
		where = append(where, "stock.freshness='FRESH' AND i.quantity_on_hand>i.quantity_reserved")
	case IndustryOutOfStock:
		where = append(where, "stock.freshness='FRESH' AND i.quantity_on_hand=i.quantity_reserved")
	case IndustryUnknown:
		where = append(where, "stock.freshness<>'FRESH'")
	default:
		return nil, nil, invalid("Некорректный фильтр наличия")
	}
	return where, args, nil
}

func (s Service) beginIndustry(ctx context.Context, t Tenant, industry string) (pgx.Tx, error) {
	tx, err := s.begin(ctx, t, false)
	if err != nil {
		return nil, err
	}
	var matches bool
	err = tx.QueryRow(ctx, "SELECT platform.industry_catalog_profile($1) IS NOT DISTINCT FROM $2::text", t.OrganizationID, industry).Scan(&matches)
	if err != nil || !matches {
		_ = tx.Rollback(ctx)
		if err != nil {
			return nil, dbError(err)
		}
		return nil, missing()
	}
	return tx, nil
}

func (s CustomerCatalog) SearchIndustry(ctx context.Context, t Tenant, f IndustrySearchFilters) (IndustrySearchResult, error) {
	return s.service.SearchIndustry(ctx, t, f)
}

func (s Service) SearchIndustry(ctx context.Context, t Tenant, f IndustrySearchFilters) (IndustrySearchResult, error) {
	where, args, err := f.predicates(t.OrganizationID)
	if err != nil {
		return IndustrySearchResult{}, err
	}
	tx, err := s.beginIndustry(ctx, t, f.Industry)
	if err != nil {
		return IndustrySearchResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL jit=off"); err != nil {
		return IndustrySearchResult{}, dbError(err)
	}
	args = append(args, f.Limit+1)
	query := `SELECT p.id,v.id,p.name,p.brand,v.sku,v.variant_name,v.price_amount,v.currency_code,stock.freshness,
 CASE WHEN stock.freshness='FRESH' THEN i.quantity_on_hand-i.quantity_reserved END
 FROM platform.product_variants v JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
 LEFT JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id
 LEFT JOIN platform.catalog_policies policy ON policy.organization_id=$1
 CROSS JOIN LATERAL (SELECT platform.inventory_freshness_at(i.manual_updated_at,coalesce(policy.fresh_hours,24),
  coalesce(policy.stale_hours,72)) AS freshness) stock
 WHERE ` + strings.Join(where, " AND ") + fmt.Sprintf(" ORDER BY v.id LIMIT $%d", len(args))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return IndustrySearchResult{}, dbError(err)
	}
	defer rows.Close()
	result := IndustrySearchResult{Items: make([]IndustrySearchItem, 0, f.Limit)}
	for rows.Next() {
		var item IndustrySearchItem
		if err = rows.Scan(&item.ProductID, &item.VariantID, &item.Name, &item.Brand, &item.SKU, &item.VariantName,
			&item.PriceAmount, &item.CurrencyCode, &item.Freshness, &item.Available); err != nil {
			return IndustrySearchResult{}, dbError(err)
		}
		if len(result.Items) == f.Limit {
			result.NextAfterVariantID = result.Items[f.Limit-1].VariantID
			break
		}
		result.Items = append(result.Items, item)
	}
	if err = rows.Err(); err != nil {
		return IndustrySearchResult{}, dbError(err)
	}
	return result, nil
}
