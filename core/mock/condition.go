package mock

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Condition struct {
	LeftTemplate string // e.g. "body.role"
	Operator     string // e.g. "==", "contains", or "" for existence
	RightValue   string // e.g. "admin", "100", or "" for existence
}

var templateExtractRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func parseCondition(expr string) *Condition {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}

	m := templateExtractRe.FindStringSubmatch(expr)
	if m == nil {
		return nil
	}
	left := m[1]

	rest := strings.TrimSpace(templateExtractRe.ReplaceAllString(expr, ""))

	if rest == "" {
		return &Condition{LeftTemplate: left, Operator: "", RightValue: ""}
	}

	type opEntry struct{ sym string }
	ops := []opEntry{{"!="}, {">="}, {"<="}, {"=="}, {">"}, {"<"}, {"contains"}, {"matches"}}

	for _, op := range ops {
		idx := strings.Index(rest, op.sym)
		if idx < 0 {
			continue
		}
		right := strings.TrimSpace(rest[idx+len(op.sym):])
		right = strings.Trim(right, "'")
		return &Condition{LeftTemplate: left, Operator: op.sym, RightValue: right}
	}

	return nil
}

func (c *Condition) Evaluate(ctx *requestContext) bool {
	if c == nil {
		return false
	}

	left := resolveValue(c.LeftTemplate, ctx)

	switch c.Operator {
	case "":
		return left != ""
	case "==":
		return left == c.RightValue
	case "!=":
		return left != c.RightValue
	case ">", "<", ">=", "<=":
		return compareNumeric(left, c.RightValue, c.Operator)
	case "contains":
		return strings.Contains(left, c.RightValue)
	case "matches":
		re, err := regexp.Compile(c.RightValue)
		if err != nil {
			return false
		}
		return re.MatchString(left)
	default:
		return false
	}
}

func compareNumeric(left, right, op string) bool {
	lv, errL := strconv.ParseFloat(left, 64)
	rv, errR := strconv.ParseFloat(right, 64)
	if errL != nil || errR != nil {
		return false
	}
	switch op {
	case ">":
		return lv > rv
	case "<":
		return lv < rv
	case ">=":
		return lv >= rv
	case "<=":
		return lv <= rv
	}
	return false
}

type Defaults struct {
	Status  int
	Body    string
	Headers map[string]string
	Delay   string
}

func resolveConditionalResponse(responses []*ConditionalResponse, ctx *requestContext, parent Defaults) (status int, body string, headers map[string]string, delay string) {
	var fallback *ConditionalResponse

	for _, r := range responses {
		if r.Default {
			fallback = r
			continue
		}

		cond := parseCondition(r.Condition)
		if cond == nil || !cond.Evaluate(ctx) {
			continue
		}

		status = r.Status
		body = r.Body
		headers = r.Headers
		delay = r.Delay

		if status == 0 {
			status = parent.Status
		}
		if body == "" {
			body = parent.Body
		}
		if len(headers) == 0 {
			headers = parent.Headers
		}
		if delay == "" {
			delay = parent.Delay
		}

		if len(r.Responses) > 0 {
			cs, cb, ch, cd := resolveConditionalResponse(r.Responses, ctx, Defaults{
				Status:  status,
				Body:    body,
				Headers: headers,
				Delay:   delay,
			})
			if cs > 0 || cb != "" {
				return cs, cb, ch, cd
			}
		}

		return status, body, headers, delay
	}

	if fallback != nil {
		s := fallback.Status
		b := fallback.Body
		h := fallback.Headers
		d := fallback.Delay
		if s == 0 {
			s = parent.Status
		}
		if b == "" {
			b = parent.Body
		}
		if len(h) == 0 {
			h = parent.Headers
		}
		if d == "" {
			d = parent.Delay
		}
		return s, b, h, d
	}

	return 0, "", nil, ""
}

func mergeHeaders(base, override map[string]string) map[string]string {
	if len(override) == 0 {
		return base
	}
	h := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		h[k] = v
	}
	for k, v := range override {
		h[k] = v
	}
	if len(h) == 0 {
		return nil
	}
	return h
}

func fmtCondition(expr string) string {
	cond := parseCondition(expr)
	if cond == nil {
		return ""
	}
	switch cond.Operator {
	case "":
		return fmt.Sprintf("{{%s}} exists", cond.LeftTemplate)
	case "!=":
		if cond.RightValue == "" {
			return fmt.Sprintf("{{%s}} exists", cond.LeftTemplate)
		}
		return fmt.Sprintf("{{%s}} != '%s'", cond.LeftTemplate, cond.RightValue)
	default:
		return fmt.Sprintf("{{%s}} %s '%s'", cond.LeftTemplate, cond.Operator, cond.RightValue)
	}
}
