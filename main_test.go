package main

import "testing"

func TestParseKV(t *testing.T) {
    k, v, ok := parseKV("country:Germany")
    if !ok || k != "country" || v != "Germany" {
        t.Fatalf("parseKV failed: %v %v %v", k, v, ok)
    }
    _, _, ok = parseKV("Germany")
    if ok { t.Fatalf("parseKV should fail for no colon") }
}

func TestMatchValueFold(t *testing.T) {
    if !matchValueFold("Germany", "ger") { t.Fatalf("should match case-insensitive substring") }
    if matchValueFold("", "*") { t.Fatalf("* should not match empty strings") }
    if !matchValueFold("France", "*") { t.Fatalf("* should match non-empty strings") }
}
