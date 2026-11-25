package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type ManagerMessage struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	RequestID string `json:"request_id,omitempty"`
}

type managerDraft struct {
	ServiceDraft bool   `json:"service_draft,omitempty"`
	Customer     string `json:"customer,omitempty"`
	SKU          string `json:"sku,omitempty"`
	Vehicle      string `json:"vehicle,omitempty"`
	Service      string `json:"service,omitempty"`
	Quantity     string `json:"quantity,omitempty"`
	Retry        bool   `json:"retry,omitempty"`
	Marketing    bool   `json:"marketing,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
}

var (
	managerRetry      = regexp.MustCompile(`(?i)(повтор|перезапуст).{0,70}(обработк|анализ|поручен|заявк)`)
	managerNoMutation = regexp.MustCompile(`(?i)(?:^|[\s,.;!?])не\s+[^.;!?]*(созда|оформ|завед|зарегистр|подготов|повтор)`)
	managerQuantity   = regexp.MustCompile(`^([1-9]|10)(?:\s*(?:шт\.?|штук[аи]?))?$`)
	managerCreate     = regexp.MustCompile(`(?i)(созда(?:й|ть|йте)|оформ(?:и|ить|ите)|завед(?:и|ите)|зарегистрируй)\s+([^;.!?]{0,70}\s+)?заявк`)
	managerMarketing  = regexp.MustCompile(`(?i)(подготов|предлож|повтор|перезапуст).{0,90}(маркетинг|черновик)`)
	managerFinance    = regexp.MustCompile(`(?i)(финанс|прибыл|выручк|поступлен|продаж)`)
	managerInventory  = regexp.MustCompile(`(?i)(остат|в наличии|артикул|sku|инвентар)`)
	managerStatus     = regexp.MustCompile(`(?i)(статус|состояни|что с заявк)`)
	managerApprovals  = regexp.MustCompile(`(?i)(согласовани|одобрен)`)
	managerIntent     = regexp.MustCompile(`(?i)(сводк|статус|остатк|финанс|согласовани|показател|сколько|клиент|продаж)`)
	managerUnsafe     = regexp.MustCompile(`(?i)(?:^|[\s,.;!?])(не\s+(созда|оформ|завед|зарегистр|подготов|повтор)|удал[иия]|спиш[иие]|отправ[ьиь]|оплат[иить]|отмени|верни средства|переведи)\b`)
	managerSlots      = regexp.MustCompile(`(?i)(?:^|[\s,;])(?:клиент(?:а)?|артикул|sku|автомобиль|машина|услуга|работа|количество|кол-во)\s*[:=]\s*|(?:^|[:,;\n])\s*(?:клиент(?:а)?|артикул|sku|автомобиль|машина|услуга|работа|количество|кол-во)\s+`)
)

func managerParse(text string, prior managerDraft, request string) (managerDraft, string) {
	// An explicit negation or an irreversible command is never dispatched from chat.
	lower := strings.ToLower(text)
	for _, phrase := range []string{"не созда", "не оформ", "не завед", "не зарегистр", "не подготов", "не повтор", "удали", "спиши", "списать", "отправь", "отправить", "оплати", "отмени", "измени", "переведи", "верни средства"} {
		if strings.Contains(lower, phrase) {
			return managerDraft{}, "unsupported"
		}
	}
	if injection.MatchString(text) || managerNoMutation.MatchString(text) || managerUnsafe.MatchString(text) {
		return managerDraft{}, "unsupported"
	}
	create, marketing := managerCreate.MatchString(text), managerMarketing.MatchString(text)
	if create && marketing {
		return managerDraft{}, "unsupported"
	}
	if marketing || prior.Marketing && !create && (request != "" || idValue.MatchString(text)) {
		if request == "" {
			request = idValue.FindString(text)
		}
		if request == "" {
			request = prior.RequestID
		}
		return managerDraft{Marketing: true, RequestID: request}, "marketing"
	}
	if (!create && managerRetry.MatchString(text)) || (prior.Retry && !create && (request != "" || idValue.MatchString(text))) {
		if request == "" {
			request = idValue.FindString(text)
		}
		return managerDraft{Retry: true, RequestID: request}, "retry"
	}
	matches := managerSlots.FindAllStringIndex(text, -1)
	if !create && (!prior.ServiceDraft || len(matches) == 0) {
		if managerIntent.MatchString(text) || managerFinance.MatchString(text) || managerInventory.MatchString(text) || managerStatus.MatchString(text) || managerApprovals.MatchString(text) {
			return managerDraft{}, "overview"
		}
		return managerDraft{}, "unsupported"
	}
	if create {
		prior = managerDraft{ServiceDraft: true}
	}
	for index, match := range matches {
		key := strings.ToLower(strings.TrimSpace(strings.Trim(text[match[0]:match[1]], " \t,:;=")))
		end := len(text)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		value := strings.TrimSpace(strings.Trim(text[match[1]:end], "\"«» :;,!?\n\r\t"))
		switch key {
		case "клиент", "клиента":
			prior.Customer = value
		case "артикул", "sku":
			prior.SKU = strings.ToUpper(value)
		case "автомобиль", "машина":
			prior.Vehicle = value
		case "услуга", "работа":
			prior.Service = value
		case "количество", "кол-во":
			prior.Quantity = value
		}
	}
	return prior, "service"
}

func (s Service) ManagerChat(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var allowed bool
	if err = tx.QueryRow(ctx, "SELECT platform.workshop_owner($1)", t.OrganizationID).Scan(&allowed); err != nil {
		return nil, tenancy.Error(err)
	}
	if !allowed {
		return nil, auth.Failure(403, "FORBIDDEN", "Кабинет управляющего доступен владельцу допущенной организации")
	}
	var out json.RawMessage
	err = tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(x ORDER BY x.created_at,x.id),'[]') FROM (
	 SELECT m.id,m.body,m.reply,m.task_id,m.created_at,j.status,j.error_code,j.correlation_id AS request_id
	 FROM platform.workshop_manager_turns m LEFT JOIN platform.agent_tasks j ON j.organization_id=m.organization_id AND j.id=m.task_id
	 WHERE m.organization_id=$1 AND m.actor_id=platform.current_user_id()
	 ORDER BY m.created_at DESC,m.id DESC LIMIT 80)x`, t.OrganizationID).Scan(&out)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	return out, nil
}

func (s Service) SendManagerMessage(ctx context.Context, t catalog.Tenant, in ManagerMessage) error {
	in.Text = strings.TrimSpace(in.Text)
	if !organizations.ValidID(in.ID) || in.Text == "" || len(in.Text) > 4000 || utf8.RuneCountInString(in.Text) > 2000 || !utf8.ValidString(in.Text) ||
		strings.ContainsFunc(in.Text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) ||
		(in.RequestID != "" && !organizations.ValidID(in.RequestID)) {
		return auth.Failure(422, "INVALID_MANAGER_MESSAGE", "Проверьте текст поручения")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var allowed bool
	if err = tx.QueryRow(ctx, "SELECT platform.workshop_owner($1)", t.OrganizationID).Scan(&allowed); err != nil {
		return tenancy.Error(err)
	}
	if !allowed {
		return auth.Failure(403, "FORBIDDEN", "Кабинет управляющего доступен владельцу допущенной организации")
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text||':'||platform.current_user_id()::text,8361))`, t.OrganizationID); err != nil {
		return tenancy.Error(err)
	}
	var existing, hint string
	err = tx.QueryRow(ctx, `SELECT body,coalesce(request_hint::text,'') FROM platform.workshop_manager_turns WHERE organization_id=$1 AND actor_id=platform.current_user_id() AND id=$2`, t.OrganizationID, in.ID).Scan(&existing, &hint)
	if err == nil {
		if existing == in.Text && hint == in.RequestID {
			return nil
		}
		return auth.Failure(409, "IDEMPOTENCY_CONFLICT", "Этот ключ сообщения уже использован")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return tenancy.Error(err)
	}
	var n int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM platform.workshop_manager_turns WHERE organization_id=$1 AND actor_id=platform.current_user_id()
	 AND created_at>clock_timestamp()-interval '1 minute'`, t.OrganizationID).Scan(&n); err != nil {
		return tenancy.Error(err)
	}
	if n >= 20 {
		return auth.Failure(429, "RATE_LIMITED", "Подождите перед следующим поручением")
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT context FROM platform.workshop_manager_turns WHERE organization_id=$1 AND actor_id=platform.current_user_id()
	 ORDER BY created_at DESC,id DESC LIMIT 1`, t.OrganizationID).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return tenancy.Error(err)
	}
	var prior managerDraft
	if len(raw) > 0 && json.Unmarshal(raw, &prior) != nil {
		return auth.Unavailable()
	}
	draft, intent := managerParse(in.Text, prior, in.RequestID)
	reply := "Эта операция недоступна через управляющего. Можно создать заявку на обслуживание, посмотреть сводку или подготовить внутреннее маркетинговое предложение по завершённой заявке."
	var task *string
	var next *managerDraft
	switch intent {
	case "overview":
		reply, err = managerOverview(ctx, tx, t.OrganizationID, in.Text, in.RequestID)
		if err != nil {
			return tenancy.Error(err)
		}
	case "retry":
		next = &draft
		reply = "Укажите заявку из списка или её ID. Повтор возможен для неудавшейся проверки до создания согласования."
		if draft.RequestID != "" {
			err = tx.QueryRow(ctx, `SELECT platform.workshop_retry_analysis($1,$2)`, t.OrganizationID, draft.RequestID).Scan(&task)
			if err != nil {
				return managerTaskError(err)
			}
			next = nil
			reply = "Для этой заявки повтор недоступен. Проверьте текущее состояние в карточке."
			if task != nil {
				reply = "Проверка повторно поставлена в очередь в той же заявке."
			}
		}
	case "marketing":
		next = &draft
		reply = "Укажите заявку в списке или добавьте её ID. Marketing нужен завершённый финансовый отчёт и действующее согласие клиента."
		if draft.RequestID != "" {
			if len(s.Key) != 32 {
				return auth.Unavailable()
			}
			err = tx.QueryRow(ctx, `SELECT platform.workshop_request_marketing($1,$2,sha256($3::bytea))`, t.OrganizationID, draft.RequestID, s.Proof()).Scan(&task)
			if err != nil {
				return managerTaskError(err)
			}
			if task != nil {
				reply = "Поручение Marketing принято. Результат и согласование появятся в карточке заявки."
				next = nil
			}
		}
	case "service":
		reply = "Уточните данные для заявки: имя клиента, артикул, автомобиль, услугу и количество от 1 до 10. Неизвестные поля можно добавить отдельным сообщением."
		next = &draft
		amount, _ := strconv.Atoi(captured(managerQuantity, strings.TrimSpace(draft.Quantity)))
		if amount >= 1 && amount <= 10 && draft.Customer != "" && draft.SKU != "" && draft.Vehicle != "" && draft.Service != "" &&
			workflowText(draft.Customer, 120) && workflowText(draft.SKU, 64) && workflowText(draft.Vehicle, 120) && workflowText(draft.Service, 160) {
			var clients, variants []string
			err = tx.QueryRow(ctx, `SELECT ARRAY(SELECT id::text FROM platform.workshop_customers WHERE organization_id=$1 AND lower(name)=lower($2) LIMIT 2),
			  ARRAY(SELECT v.id::text FROM platform.product_variants v JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
			   WHERE v.organization_id=$1 AND v.sku=$3 AND p.status='active' LIMIT 2)`, t.OrganizationID, draft.Customer, draft.SKU).Scan(&clients, &variants)
			if err != nil {
				return tenancy.Error(err)
			}
			switch {
			case len(clients) != 1:
				reply = "Уточните точное уникальное имя клиента из карточек организации."
			case len(variants) != 1:
				reply = "Уточните артикул активной детали из каталога вашей организации."
			default:
				if len(s.Key) != 32 {
					return auth.Unavailable()
				}
				err = tx.QueryRow(ctx, `SELECT platform.workshop_submit($1,$2,$3,$4,$5,$6,$7,sha256($8::bytea))`,
					t.OrganizationID, in.ID, clients[0], variants[0], amount, draft.Vehicle, draft.Service, s.Proof()).Scan(&task)
				if err != nil {
					return managerTaskError(err)
				}
				next = nil
				reply = "Заявка передана Customer, Product & Inventory и Sales. Согласование резерва появится в карточке после проверок."
			}
		}
	}
	var contextBytes []byte
	if next != nil {
		contextBytes, err = json.Marshal(next)
		if err != nil {
			return auth.Unavailable()
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform.workshop_manager_turns(organization_id,actor_id,id,body,reply,request_hint,context,task_id)
	  VALUES($1,platform.current_user_id(),$2,$3,$4,nullif($5,'')::uuid,$6::jsonb,$7)`, t.OrganizationID, in.ID, in.Text, reply, in.RequestID, contextBytes, task)
	if err != nil {
		return tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return tenancy.Error(err)
	}
	return nil
}

func managerTaskError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "54000" {
		return auth.Failure(429, "RATE_LIMITED", "Слишком много поручений")
	}
	return tenancy.Error(err)
}

func managerOverview(ctx context.Context, tx pgx.Tx, org, text, hint string) (string, error) {
	switch {
	case managerStatus.MatchString(text):
		if hint == "" {
			hint = idValue.FindString(text)
		}
		if hint != "" {
			var service, state string
			err := tx.QueryRow(ctx, `SELECT r.service,CASE WHEN r.close_kind='consume' THEN 'обслуживание завершено'
     WHEN r.close_kind='release' THEN 'отменена' WHEN r.reserved_at IS NOT NULL THEN 'деталь в резерве'
     WHEN a.status='pending' AND a.expires_at>clock_timestamp() THEN 'ожидает согласования' ELSE coalesce(a.status,j.status) END
     FROM platform.workshop_requests r JOIN platform.agent_tasks j ON j.organization_id=r.organization_id AND j.id=r.id
     LEFT JOIN platform.agent_approvals a ON a.organization_id=r.organization_id AND a.task_id=r.reserve_task_id
     WHERE r.organization_id=$1 AND r.id=$2`, org, hint).Scan(&service, &state)
			if errors.Is(err, pgx.ErrNoRows) {
				return "Заявку не удалось найти в вашей организации.", nil
			}
			if err != nil {
				return "", err
			}
			labels := map[string]string{"queued": "в очереди", "running": "проверяется", "completed": "проверка завершена", "failed": "не удалось обработать", "expired": "срок истёк", "rejected": "отклонено", "approved": "согласовано", "execution_failed": "резерв не оформлен", "cancelled": "отменена"}
			if label := labels[state]; label != "" {
				state = label
			}
			return fmt.Sprintf("Заявка «%s» %s. Детали в карточке заявки.", service, state), nil
		}
		return "Укажите ID заявки, чтобы уточнить её состояние.", nil
	case managerFinance.MatchString(text):
		var count, paid, cash, gross int64
		err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE paid_at IS NOT NULL),
   coalesce(sum(service_minor) FILTER(WHERE paid_at IS NOT NULL),0),coalesce(sum(service_minor-part_cost_minor),0)
   FROM platform.workshop_financials WHERE organization_id=$1`, org).Scan(&count, &paid, &cash, &gross)
		if err != nil {
			return "", err
		}
		if count == 0 {
			return "Подтверждённых финансовых данных пока нет.", nil
		}
		return fmt.Sprintf("Учтено заявок %d, из них оплачено %d. Получено %s. Валовая прибыль по учтённым заявкам %s.", count, paid, managerMoney(cash), managerMoney(gross)), nil
	case managerInventory.MatchString(text):
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM platform.inventory i JOIN platform.product_variants v ON v.organization_id=i.organization_id AND v.id=i.variant_id
   JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
   WHERE i.organization_id=$1 AND p.status='active' AND i.quantity_on_hand>i.quantity_reserved
    AND platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH'`, org).Scan(&count)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Активных артикулов со свежим доступным остатком: %d. Выбранную деталь проверит Product & Inventory в заявке.", count), nil
	case managerApprovals.MatchString(text):
		var pending int
		err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform.agent_approvals WHERE organization_id=$1 AND status='pending' AND expires_at>clock_timestamp())+
   (SELECT count(*) FROM platform.marketing_drafts d JOIN platform.agent_tasks c ON c.organization_id=d.organization_id AND c.id=d.id
   JOIN platform.agent_tasks m ON m.organization_id=c.organization_id AND m.id=c.parent_id AND m.status='completed'
   WHERE d.organization_id=$1 AND d.status='pending')`, org).Scan(&pending)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Ожидают решения: %d. Откройте согласования в кабинете.", pending), nil
	default:
		var customers, requests, completed int64
		err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform.workshop_customers WHERE organization_id=$1),
   (SELECT count(*) FROM platform.workshop_requests WHERE organization_id=$1),
   (SELECT count(*) FROM platform.workshop_requests WHERE organization_id=$1 AND close_kind='consume')`, org).Scan(&customers, &requests, &completed)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Клиентов: %d. Заявок: %d. Завершены со списанием: %d. Подробности в сводке кабинета.", customers, requests, completed), nil
	}
}

func managerMoney(minor int64) string {
	if minor < 0 {
		return fmt.Sprintf("−%d,%02d ₽", -(minor / 100), -(minor % 100))
	}
	return fmt.Sprintf("%d,%02d ₽", minor/100, minor%100)
}
