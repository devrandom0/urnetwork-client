package main

import "testing"

func TestDomainMatches(t *testing.T) {
    if !domainMatches("api.example.com", []string{"example.com"}) {
        t.Fatalf("suffix should match")
    }
    if domainMatches("api.other.com", []string{"example.com"}) {
        t.Fatalf("unexpected match")
    }
    if !domainMatches("example.com.", []string{"example.com"}) {
        t.Fatalf("trailing dot should be ignored")
    }
}
