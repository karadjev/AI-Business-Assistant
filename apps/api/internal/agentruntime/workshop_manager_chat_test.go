package agentruntime

import "testing"

func TestManagerIntentAndClarification(t *testing.T) {
	draft, intent := managerParse("Можете оформить новую заявку на обслуживание?", managerDraft{}, "")
	if intent != "service" || !draft.ServiceDraft || draft.Customer != "" {
		t.Fatalf("missing data: %+v %s", draft, intent)
	}
	first, intent := managerParse("Оформи заявку: клиент Иванов; артикул abc-1", managerDraft{}, "")
	if intent != "service" || first.Customer != "Иванов" || first.SKU != "ABC-1" {
		t.Fatalf("initial slots: %+v %s", first, intent)
	}
	next, intent := managerParse("Автомобиль: Условный автомобиль 2021; услуга: замена фильтра; количество: 2 шт.", first, "")
	if intent != "service" || next.SKU != "ABC-1" || next.Vehicle != "Условный автомобиль 2021" || next.Service != "замена фильтра" || captured(managerQuantity, next.Quantity) != "2" {
		t.Fatalf("follow-up: %+v %s", next, intent)
	}
	for _, message := range []string{"Не нужно создавать заявку", "Оплати заявку", "Отправь сообщение и подготовь маркетинг", "Купи автомобиль"} {
		if _, intent = managerParse(message, next, ""); intent != "unsupported" {
			t.Fatalf("%q routed to %s", message, intent)
		}
	}
	if _, intent = managerParse("Покажите сводку", managerDraft{Marketing: true}, ""); intent != "overview" {
		t.Fatal("explicit overview overwritten by clarification")
	}
}
