package orders

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

func checkHybridAvailability(items []Snapshot) error {
	for _, item := range items {
		if !item.Fresh || item.Available < item.Quantity {
			return failure("INVENTORY_UNVERIFIED", "Подтверждённого остатка для заявки недостаточно")
		}
	}
	return nil
}

func (s Service) saveHybridItems(ctx context.Context, tx pgx.Tx, t catalog.Tenant, orderID string, cart []CartItem) error {
	items, err := s.lockCatalog(ctx, tx, cart, true)
	if err != nil {
		return err
	}
	for _, item := range items {
		_, err = tx.Exec(ctx, `INSERT INTO platform.order_items
		(id,organization_id,order_id,variant_id,product_id,quantity,unit_price_amount,currency_code,name_snapshot,sku_snapshot)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			auth.UUID(), t.OrganizationID, orderID, item.VariantID, item.ProductID, item.Quantity, item.UnitPrice, item.Currency, item.Name, item.SKU)
		if err != nil {
			return tenancy.Error(err)
		}
	}
	return nil
}

func (s Service) changeHybrid(ctx context.Context, tx pgx.Tx, t catalog.Tenant, id, target, previous string, selection servicebooking.Selection) error {
	org := t.OrganizationID
	switch {
	case target == "confirmed":
		if err := kindAllowed(ctx, tx, org, "hybrid_order"); err != nil {
			return err
		}
		if err := s.changeProductStock(ctx, tx, t, id, target, previous); err != nil {
			return err
		}
		if err := servicebooking.ConfirmTx(ctx, tx, org, id, selection); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO platform.hybrid_reservations(organization_id,order_id,expires_at)
		 SELECT organization_id,request_id,ends_at+interval '24 hours' FROM platform.service_bookings
		 WHERE organization_id=$1 AND request_id=$2 AND status='confirmed'`, org, id)
		if err != nil {
			return tenancy.Error(err)
		}
		if tag.RowsAffected() != 1 {
			return failure("RESERVATION_CONFLICT", "Не удалось подтвердить резерв")
		}
		return nil
	case previous == "confirmed" && (target == "cancelled" || target == "completed"):
		state := "released"
		if target == "completed" {
			state = "consumed"
			var ended bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.service_bookings
			 WHERE organization_id=$1 AND request_id=$2 AND status='confirmed' AND ends_at<=clock_timestamp())`, org, id).Scan(&ended)
			if err != nil {
				return tenancy.Error(err)
			}
			if !ended {
				return failure("SERVICE_NOT_ENDED", "Запись ещё не завершилась")
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE platform.hybrid_reservations SET status=$3
		 WHERE organization_id=$1 AND order_id=$2 AND status='active'
		 AND ($3<>'consumed' OR expires_at>clock_timestamp())`, org, id, state)
		if err != nil {
			return tenancy.Error(err)
		}
		if tag.RowsAffected() != 1 {
			return failure("RESERVATION_CONFLICT", "Резерв уже завершён или требует проверки")
		}
		if err = s.changeProductStock(ctx, tx, t, id, target, previous); err != nil {
			return err
		}
		if target == "cancelled" {
			return servicebooking.CancelTx(ctx, tx, org, id)
		}
		return servicebooking.CompleteHybridTx(ctx, tx, org, id)
	}
	return nil
}

func (s Service) ExpireOne(ctx context.Context) (bool, error) {
	var expired bool
	err := s.Pool.QueryRow(ctx, "SELECT platform.expire_hybrid_reservation()").Scan(&expired)
	return expired, err
}
