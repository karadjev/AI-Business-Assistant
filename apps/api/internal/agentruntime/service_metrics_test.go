package agentruntime

import "testing"

func TestServiceMetrics(t *testing.T) {
	values, err := serviceMetrics([]byte(`{"completed_count":1,"paid_count":0,"received_minor":null,"cost_minor":null,"difference_minor":null}`))
	if err != nil || len(values) != 2 || values[0].Value != 1 || values[1].Value != 0 {
		t.Fatalf("completed unpaid service: %v, %v", values, err)
	}
	values, err = serviceMetrics([]byte(`{"completed_count":1,"paid_count":1,"received_minor":1000,"cost_minor":1100,"difference_minor":-100}`))
	if err != nil || len(values) != 5 || values[2].Value != 1000 || values[3].Value != 1100 || values[4].Value != -100 {
		t.Fatalf("paid service: %v, %v", values, err)
	}
	if _, err = serviceMetrics([]byte(`{"completed_count":1,"paid_count":1}`)); err == nil {
		t.Fatal("missing confirmed amounts must fail")
	}
}
