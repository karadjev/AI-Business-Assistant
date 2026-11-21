package catalog

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
)

// MoveOrderStock participates in the caller's tenant transaction. Caller locks
// the order, all products in sorted ID order, then inventory in sorted variant order.
// There is deliberately no public reserve endpoint.
func MoveOrderStock(ctx context.Context, tx pgx.Tx, t Tenant, orderID, variantID, kind string, quantity int) error {
	if quantity < 1 || quantity > 10 {
		return invalid("Количество: 1–10")
	}
	var on, reserved, version int64
	e := tx.QueryRow(ctx, `SELECT quantity_on_hand,quantity_reserved,version FROM platform.inventory WHERE variant_id=$1 FOR UPDATE`, variantID).Scan(&on, &reserved, &version)
	if e != nil {
		return dbError(e)
	}
	nextOn, nextReserved := on, reserved
	switch kind {
	case "reserve":
		var freshness string
		if e = tx.QueryRow(ctx, `SELECT platform.inventory_freshness(organization_id,manual_updated_at) FROM platform.inventory WHERE variant_id=$1`, variantID).Scan(&freshness); e != nil {
			return dbError(e)
		}
		if freshness != "FRESH" {
			return auth.Failure(409, "STOCK_STALE", "Наличие требует свежей сверки магазина")
		}

		if on-reserved < int64(quantity) {
			return auth.Failure(409, "INSUFFICIENT_STOCK", "Недостаточно доступного товара")
		}
		nextReserved += int64(quantity)
	case "release":
		if reserved < int64(quantity) {
			return auth.Failure(409, "RESERVATION_CONFLICT", "Резерв требует проверки магазина")
		}
		nextReserved -= int64(quantity)
	case "consume":
		if reserved < int64(quantity) {
			return auth.Failure(409, "RESERVATION_CONFLICT", "Резерв требует проверки магазина")
		}
		nextReserved -= int64(quantity)
		nextOn -= int64(quantity)
	default:
		return invalid("Некорректное движение")
	}
	if _, e = tx.Exec(ctx, `SELECT set_config('app.catalog_stock_move','true',true)`); e != nil {
		return dbError(e)
	}
	tag, e := tx.Exec(ctx, `UPDATE platform.inventory SET quantity_on_hand=$2,quantity_reserved=$3,version=version+1,updated_at=now() WHERE variant_id=$1 AND version=$4`, variantID, nextOn, nextReserved, version)
	if e == nil {
		_, e = tx.Exec(ctx, `SELECT set_config('app.catalog_stock_move','false',true)`)
	}
	if e != nil {
		return dbError(e)
	}
	if tag.RowsAffected() != 1 {
		return conflict()
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform.inventory_movements(id,organization_id,variant_id,actor_id,reason,quantity_before,quantity_after,delta,inventory_version,kind,reserved_before,reserved_after,delta_reserved,order_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, auth.UUID(), t.OrganizationID, variantID, t.Session.User.ID, "Заявка: "+kind, on, nextOn, nextOn-on, version+1, kind, reserved, nextReserved, nextReserved-reserved, orderID)
	if e != nil {
		return dbError(e)
	}
	return nil
}
