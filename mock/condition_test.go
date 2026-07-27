package mock

import "testing"

func eqCtx() *requestContext {
	return &requestContext{
		query:  map[string]string{"page": "3", "size": "50"},
		path:   map[string]string{"id": "42"},
		header: map[string]string{"authorization": "Bearer xxx", "accept": "application/json"},
		body:   map[string]interface{}{"name": "alice", "role": "user", "amount": 250.5, "email": "alice@example.com"},
	}
}

func TestParseConditionEquality(t *testing.T) {
	c := parseCondition(`{{body.role}} == 'admin'`)
	if c == nil {
		t.Fatal("expected condition")
	}
	if c.Operator != "==" {
		t.Errorf("expected ==, got %s", c.Operator)
	}
	if c.RightValue != "admin" {
		t.Errorf("expected admin, got %s", c.RightValue)
	}
}

func TestParseConditionExistence(t *testing.T) {
	c := parseCondition(`{{header.authorization}}`)
	if c == nil {
		t.Fatal("expected condition")
	}
	if c.Operator != "" {
		t.Errorf("expected empty operator, got %s", c.Operator)
	}
}

func TestParseConditionContains(t *testing.T) {
	c := parseCondition(`{{header.accept}} contains 'json'`)
	if c == nil {
		t.Fatal("expected condition")
	}
	if c.Operator != "contains" {
		t.Errorf("expected contains, got %s", c.Operator)
	}
	if c.RightValue != "json" {
		t.Errorf("expected json, got %s", c.RightValue)
	}
}

func TestEvaluateEqTrue(t *testing.T) {
	c := &Condition{LeftTemplate: "body.role", Operator: "==", RightValue: "user"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for body.role == 'user'")
	}
}

func TestEvaluateEqFalse(t *testing.T) {
	c := &Condition{LeftTemplate: "body.role", Operator: "==", RightValue: "admin"}
	if c.Evaluate(eqCtx()) {
		t.Error("expected false for body.role == 'admin'")
	}
}

func TestEvaluateNeq(t *testing.T) {
	c := &Condition{LeftTemplate: "body.role", Operator: "!=", RightValue: "admin"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for body.role != 'admin'")
	}
}

func TestEvaluateExists(t *testing.T) {
	c := &Condition{LeftTemplate: "header.authorization", Operator: "", RightValue: ""}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for authorization exists")
	}
}

func TestEvaluateNotExists(t *testing.T) {
	c := &Condition{LeftTemplate: "header.x-nonexistent", Operator: "", RightValue: ""}
	if c.Evaluate(eqCtx()) {
		t.Error("expected false for nonexistent header")
	}
}

func TestEvaluateGreater(t *testing.T) {
	c := &Condition{LeftTemplate: "body.amount", Operator: ">", RightValue: "100"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for 250 > 100")
	}
}

func TestEvaluateGreaterFalse(t *testing.T) {
	c := &Condition{LeftTemplate: "body.amount", Operator: ">", RightValue: "300"}
	if c.Evaluate(eqCtx()) {
		t.Error("expected false for 250 > 300")
	}
}

func TestEvaluateLess(t *testing.T) {
	c := &Condition{LeftTemplate: "body.amount", Operator: "<", RightValue: "300"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for 250 < 300")
	}
}

func TestEvaluateContainsTrue(t *testing.T) {
	c := &Condition{LeftTemplate: "header.accept", Operator: "contains", RightValue: "json"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for accept contains 'json'")
	}
}

func TestEvaluateContainsFalse(t *testing.T) {
	c := &Condition{LeftTemplate: "header.accept", Operator: "contains", RightValue: "xml"}
	if c.Evaluate(eqCtx()) {
		t.Error("expected false for accept contains 'xml'")
	}
}

func TestEvaluateMatchesTrue(t *testing.T) {
	c := &Condition{LeftTemplate: "body.email", Operator: "matches", RightValue: ".*@example.com$"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for email matches regex")
	}
}

func TestEvaluateMatchesFalse(t *testing.T) {
	c := &Condition{LeftTemplate: "body.email", Operator: "matches", RightValue: ".*@gmail.com$"}
	if c.Evaluate(eqCtx()) {
		t.Error("expected false for email not matching regex")
	}
}

func TestEvaluatePathParam(t *testing.T) {
	c := &Condition{LeftTemplate: "path.id", Operator: "==", RightValue: "42"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for path.id == '42'")
	}
}

func TestEvaluateQueryParam(t *testing.T) {
	c := &Condition{LeftTemplate: "query.page", Operator: "==", RightValue: "3"}
	if !c.Evaluate(eqCtx()) {
		t.Error("expected true for query.page == '3'")
	}
}

func TestParseEmpty(t *testing.T) {
	c := parseCondition("")
	if c != nil {
		t.Error("expected nil for empty expression")
	}
}

func TestParseInvalid(t *testing.T) {
	c := parseCondition("not a template")
	if c != nil {
		t.Error("expected nil for invalid expression")
	}
}

func TestParseNestedBody(t *testing.T) {
	c := parseCondition(`{{body.user.address.city}} == 'Shanghai'`)
	if c == nil {
		t.Fatal("expected condition")
	}
	if c.LeftTemplate != "body.user.address.city" {
		t.Errorf("expected body.user.address.city, got %s", c.LeftTemplate)
	}
	if c.RightValue != "Shanghai" {
		t.Errorf("expected Shanghai, got %s", c.RightValue)
	}
}
